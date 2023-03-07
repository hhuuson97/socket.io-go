package sio

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/tomruk/socket.io-go/parser"
)

type NamespaceConnectFunc func(socket ServerSocket)

type Namespace struct {
	name   string
	server *Server

	sockets *NamespaceSocketStore

	middlewareFuncs   []NspMiddlewareFunc
	middlewareFuncsMu sync.RWMutex

	adapter Adapter
	parser  parser.Parser

	ackID uint64
	ackMu sync.Mutex

	emitterForEvents  *eventEmitter[*eventHandler]
	emitterForConnect *eventEmitter[*NamespaceConnectFunc]
}

func newNamespace(name string, server *Server, adapterCreator AdapterCreator, parserCreator parser.Creator) *Namespace {
	socketStore := newNamespaceSocketStore()
	nsp := &Namespace{
		name:             name,
		server:           server,
		sockets:          socketStore,
		parser:           parserCreator(),
		emitterForEvents: newEventEmitter[*eventHandler](),
	}
	nsp.adapter = adapterCreator(nsp, socketStore, parserCreator)
	return nsp
}

func (n *Namespace) Name() string { return n.name }

func (n *Namespace) Adapter() Adapter { return n.adapter }

type NspMiddlewareFunc func(socket ServerSocket, handshake *Handshake) error

func (n *Namespace) Use(f NspMiddlewareFunc) {
	n.middlewareFuncsMu.Lock()
	defer n.middlewareFuncsMu.Unlock()
	n.middlewareFuncs = append(n.middlewareFuncs, f)
}

func (n *Namespace) OnEvent(eventName string, handler any) {
	n.checkHandler(eventName, handler)
	n.emitterForEvents.On(eventName, newEventHandler(handler))
}

func (n *Namespace) OnceEvent(eventName string, handler any) {
	n.checkHandler(eventName, handler)
	n.emitterForEvents.Once(eventName, newEventHandler(handler))
}

func (n *Namespace) checkHandler(eventName string, handler any) {
	switch eventName {
	case "":
		fallthrough
	case "connect":
		fallthrough
	case "connection":
		err := checkNamespaceHandler(eventName, handler)
		if err != nil {
			panic(fmt.Errorf("sio: %w", err))
		}
	}
}

func (n *Namespace) OffEvent(eventName string, _handler ...any) {
	handlers := make([]*eventHandler, len(_handler))
	for i, h := range _handler {
		handlers[i] = newEventHandler(h)
	}
	n.emitterForEvents.Off(eventName, handlers...)
}

func (n *Namespace) OffAll() {
	n.emitterForEvents.OffAll()
}

// Emits an event to all connected clients in the given namespace.
func (n *Namespace) Emit(eventName string, v ...any) {
	newBroadcastOperator(n.Name(), n.adapter, n.parser).Emit(eventName, v...)
}

// Sends a message to the other Socket.IO servers of the cluster.
func (n *Namespace) ServerSideEmit(eventName string, _v ...any) {
	header := &parser.PacketHeader{
		Type:      parser.PacketTypeEvent,
		Namespace: n.Name(),
	}

	if IsEventReservedForServer(eventName) {
		panic("sio: broadcastOperator.Emit: attempted to emit to a reserved event")
	}

	// One extra space for eventName,
	// the other for ID (see the Broadcast method of sessionAwareAdapter)
	v := make([]any, 0, len(_v)+1)
	v = append(v, eventName)
	v = append(v, _v...)

	n.adapter.ServerSideEmit(header, v)
}

func (n *Namespace) OnServerSideEmit(eventName string, _v ...any) {
	values := make([]reflect.Value, len(_v))
	for i, v := range _v {
		values[i] = reflect.ValueOf(v)
	}
	handlers := n.emitterForEvents.GetHandlers(eventName)

	go func() {
		for _, handler := range handlers {
			if len(values) == len(handler.inputArgs) {
				for i, v := range values {
					if handler.inputArgs[i].Kind() != reflect.Ptr && v.Kind() == reflect.Ptr {
						values[i] = v.Elem()
					}
				}
			} else {
				// TODO: Error?
				return
			}
			handler.Call(values...)
		}
	}()
}

// Sets a modifier for a subsequent event emission that the event
// will only be broadcast to clients that have joined the given room.
//
// To emit to multiple rooms, you can call `To` several times.
func (n *Namespace) To(room ...Room) *BroadcastOperator {
	return newBroadcastOperator(n.Name(), n.adapter, n.parser).To(room...)
}

// Alias of To(...)
func (n *Namespace) In(room ...Room) *BroadcastOperator {
	return newBroadcastOperator(n.Name(), n.adapter, n.parser).In(room...)
}

// Sets a modifier for a subsequent event emission that the event
// will only be broadcast to clients that have not joined the given rooms.
func (n *Namespace) Except(room ...Room) *BroadcastOperator {
	return newBroadcastOperator(n.Name(), n.adapter, n.parser).Except(room...)
}

// Compression flag is unused at the moment, thus setting this will have no effect on compression.
func (n *Namespace) Compress(compress bool) *BroadcastOperator {
	return newBroadcastOperator(n.Name(), n.adapter, n.parser).Compress(compress)
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node (when scaling to multiple nodes).
//
// See: https://socket.io/docs/v4/using-multiple-nodes
func (n *Namespace) Local() *BroadcastOperator {
	return newBroadcastOperator(n.Name(), n.adapter, n.parser).Local()
}

// Gets the sockets of the namespace.
// Beware that this is local to the current node. For sockets across all nodes, use FetchSockets
func (n *Namespace) Sockets() []ServerSocket {
	return n.sockets.GetAll()
}

// Returns the matching socket instances. This method works across a cluster of several Socket.IO servers.
func (n *Namespace) FetchSockets() []AdapterSocket {
	return newBroadcastOperator(n.Name(), n.adapter, n.parser).FetchSockets()
}

// Makes the matching socket instances join the specified rooms.
func (n *Namespace) SocketsJoin(room ...Room) {
	newBroadcastOperator(n.Name(), n.adapter, n.parser).SocketsJoin(room...)
}

// Makes the matching socket instances leave the specified rooms.
func (n *Namespace) SocketsLeave(room ...Room) {
	newBroadcastOperator(n.Name(), n.adapter, n.parser).SocketsLeave(room...)
}

// Makes the matching socket instances disconnect from the namespace.
//
// If value of close is true, closes the underlying connection. Otherwise, it just disconnects the namespace.
func (n *Namespace) DisconnectSockets(close bool) {
	newBroadcastOperator(n.Name(), n.adapter, n.parser).DisconnectSockets(close)
}

type authRecoveryFields struct {
	SessionID string
	Offset    string
}

func (n *Namespace) add(c *serverConn, auth json.RawMessage) (*serverSocket, error) {
	var (
		handshake = &Handshake{
			Time: time.Now(),
			Auth: auth,
		}
		authRecoveryFields authRecoveryFields
		socket             *serverSocket
	)

	err := json.Unmarshal(auth, &authRecoveryFields)
	if err != nil {
		return nil, err
	}

	if n.server.connectionStateRecovery.Enabled {
		session := n.adapter.RestoreSession(PrivateSessionID(authRecoveryFields.SessionID), authRecoveryFields.Offset)
		if session != nil {
			socket, err = newServerSocket(n.server, c, n, c.parser, session)
			if err != nil {
				return nil, err
			}
		}
	}

	// If connection state recovery is disabled
	// or for some reason socket couldn't be retrieved
	if socket == nil {
		socket, err = newServerSocket(n.server, c, n, c.parser, nil)
		if err != nil {
			return nil, err
		}
	}

	if n.server.connectionStateRecovery.Enabled && !n.server.connectionStateRecovery.UseMiddlewares && socket.Recovered() {
		return socket, n.doConnect(socket)
	}

	err = n.runMiddlewares(socket, handshake)
	if err != nil {
		return nil, err
	}

	return socket, n.doConnect(socket)
}

func (n *Namespace) runMiddlewares(socket *serverSocket, handshake *Handshake) error {
	n.middlewareFuncsMu.RLock()
	defer n.middlewareFuncsMu.RUnlock()

	for _, f := range n.middlewareFuncs {
		err := f(socket, handshake)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *Namespace) doConnect(socket *serverSocket) error {
	n.sockets.Set(socket)

	// It is paramount that the internal `onconnect` logic
	// fires before user-set events to prevent state order
	// violations (such as a disconnection before the connection
	// logic is complete)
	err := socket.onConnect()
	if err != nil {
		return err
	}

	connectHandlers := n.emitterForConnect.GetHandlers("connect")

	go func() {
		for _, handler := range connectHandlers {
			(*handler)(socket)
		}
	}()
	return nil
}

func (n *Namespace) remove(socket *serverSocket) {
	n.sockets.Remove(socket.ID())
}

func (n *Namespace) nextAckID() uint64 {
	n.ackMu.Lock()
	defer n.ackMu.Unlock()
	id := n.ackID
	n.ackID++
	return id
}
