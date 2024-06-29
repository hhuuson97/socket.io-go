package sio

import (
	"net/http"
	"testing"
	"time"

	eio "github.com/tomruk/socket.io-go/engine.io"
	"github.com/tomruk/socket.io-go/internal/sync"
	"github.com/tomruk/socket.io-go/internal/utils"

	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {
	t.Run("should authenticate", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(t, nil, nil)
		socket := manager.Socket("/", nil).(*clientSocket)

		type S struct {
			Num int
		}
		s := &S{
			Num: 500,
		}

		err := socket.setAuth(s)
		if err != nil {
			t.Fatal(err)
		}

		s, ok := socket.Auth().(*S)
		assert.True(t, ok)
		assert.Equal(t, s.Num, 500)

		err = socket.setAuth("Donkey")
		assert.NotNil(t, err)

		assert.PanicsWithError(t, "sio: SetAuth: non-JSON data cannot be accepted. please provide a struct or map", func() {
			socket.SetAuth("Donkey")
		})

		close()
	})

	t.Run("should connect to a namespace after connection established", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			nil,
		)
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/", nil)

		socket.OnConnect(func() {
			t.Log("/ connected")
			asdf := manager.Socket("/asdf", nil)
			asdf.OnConnect(func() {
				t.Log("/asdf connected")
				tw.Done()
			})
			asdf.Connect()
		})
		socket.Connect()

		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})

	t.Run("should be able to connect to a new namespace after connection gets closed", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			nil,
		)
		socket := manager.Socket("/", nil)
		tw := utils.NewTestWaiter(1)
		done := sync.OnceFunc(func() { tw.Done() })

		socket.OnConnect(func() {
			t.Log("/ connected")
			socket.Disconnect()
		})
		socket.OnDisconnect(func(reason Reason) {
			t.Logf("/ disconnected with reason: %s", reason)
			asdf := manager.Socket("/asdf", nil)
			asdf.OnConnect(func() {
				t.Log("/asdf connected")
				done()
			})
			t.Log("/asdf is connecting")
			asdf.Connect()
		})
		socket.Connect()

		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})

	t.Run("manager open without socket", func(t *testing.T) {
		server, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
				ConnectTimeout:     1000 * time.Millisecond,
			},
			&ManagerConfig{
				NoReconnection: true,
			},
		)
		tw := utils.NewTestWaiterString()
		tw.Add("OnOpen")
		tw.Add("OnClose")

		server.OnAnyConnection(func(namespace string, socket ServerSocket) {
			t.Fatalf("Connection to `%s` was received. This shouldn't have happened", namespace)
		})

		manager.OnOpen(func() {
			t.Log("Manager connection is established")
			tw.Done("OnOpen")
		})
		manager.OnClose(func(reason Reason, err error) {
			assert.Equal(t, Reason("transport close"), reason)
			tw.Done("OnClose")
		})
		manager.Open()

		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})

	t.Run("should reconnect by default", func(t *testing.T) {
		server, _, manager, close := newTestServerAndClient(
			t,
			nil,
			nil,
		)
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/", nil)

		server.OnConnection(func(socket ServerSocket) {
			s := socket.(*serverSocket)
			// Abruptly close the connection.
			s.conn.eio.Close()
		})
		manager.OnReconnect(func(attempt uint32) {
			socket.Disconnect()
			tw.Done()
		})

		socket.Connect()
		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})

	t.Run("should reconnect manually", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				NoReconnection: true,
			},
		)
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/", nil)

		socket.OnceConnect(func() {
			socket.Disconnect()
		})
		socket.OnceDisconnect(func(reason Reason) {
			socket.OnceConnect(func() {
				socket.Disconnect()
				tw.Done()
			})
			socket.Connect()
		})

		socket.Connect()
		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})

	t.Run("should reconnect automatically after reconnecting manually", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			nil,
		)
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/", nil)

		socket.OnceConnect(func() {
			socket.Disconnect()
		})
		socket.OnceDisconnect(func(reason Reason) {
			socket.Manager().OnceReconnect(func(attempt uint32) {
				socket.Disconnect()
				tw.Done()
			})
			socket.Connect()
			time.Sleep(500 * time.Millisecond)
			socket.Manager().eioMu.Lock()
			defer socket.Manager().eioMu.Unlock()
			// Call inside another goroutine to prevent eioMu to be locked more than once at the same goroutine.
			go socket.Manager().eio.Close()
		})

		socket.Connect()
		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})

	t.Run("should attempt reconnects after a failed reconnect", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				ReconnectionAttempts: 2,
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		close() // To force reconnect by preventing client from connecting.
		tw := utils.NewTestWaiter(1)

		socket := manager.Socket("/timeout", nil)
		manager.OnceReconnectFailed(func() {
			var (
				reconnects = 0
				mu         sync.Mutex
			)
			manager.OnReconnectAttempt(func(attempt uint32) {
				mu.Lock()
				reconnects++
				mu.Unlock()
			})
			manager.OnReconnectFailed(func() {
				mu.Lock()
				assert.Equal(t, 2, reconnects)
				mu.Unlock()
				socket.Disconnect()
				manager.Close()
				tw.Done()
			})
			socket.Connect()
		})
		socket.Connect()

		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
	})

	t.Run("should stop reconnecting when force closed", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		tw := utils.NewTestWaiter(1)
		close() // To force error by preventing client from connecting.
		socket := manager.Socket("/", nil)
		manager.OnceReconnectAttempt(func(attempt uint32) {
			socket.Disconnect()
			manager.OnReconnectAttempt(func(attempt uint32) {
				t.FailNow()
			})
			time.Sleep(500 * time.Millisecond)
			tw.Done()
		})
		socket.Connect()

		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
	})

	t.Run("should reconnect after stopping reconnection", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		tw := utils.NewTestWaiter(1)
		close() // To force error by preventing client from connecting.
		socket := manager.Socket("/", nil)
		manager.OnceReconnectAttempt(func(attempt uint32) {
			manager.OnReconnectAttempt(func(attempt uint32) {
				socket.Disconnect()
				tw.Done()
			})
			socket.Disconnect()
			socket.Connect()
			tw.Done()
		})
		socket.Connect()

		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
	})

	t.Run("should stop reconnecting on a socket and keep to reconnect on another", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		io, ts, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
				EIO: eio.ServerConfig{
					PingTimeout:  3000 * time.Millisecond,
					PingInterval: 1000 * time.Millisecond,
				},
			},
			&ManagerConfig{
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		tw := utils.NewTestWaiter(1)
		socket1 := manager.Socket("/", nil)
		socket2 := manager.Socket("/asd", nil)
		manager.OnceReconnectAttempt(func(attempt uint32) {
			socket1.OnConnect(func() {
				t.FailNow()
			})
			socket2.OnConnect(func() {
				time.Sleep(500 * time.Millisecond)
				socket2.Disconnect()
				manager.Close()
				tw.Done()
			})
			socket1.Disconnect()
		})
		socket1.Connect()
		socket2.Connect()

		go func() {
			time.Sleep(1000 * time.Millisecond)
			ts.Close()
			time.Sleep(5000 * time.Millisecond)
			hs := http.Server{
				Addr:    ts.Listener.Addr().String(),
				Handler: io,
			}
			err := hs.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				panic(err)
			}
		}()

		tw.WaitTimeout(t, 20*time.Second)
		close()
	})

	t.Run("should try to reconnect twice and fail when requested two attempts with immediate timeout and reconnect enabled", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				ReconnectionAttempts: 2,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		close()
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/timeout", nil)
		reconnects := 0
		reconnectsMu := sync.Mutex{}

		manager.OnReconnectAttempt(func(attempt uint32) {
			reconnectsMu.Lock()
			reconnects++
			reconnectsMu.Unlock()
		})
		manager.OnReconnectFailed(func() {
			reconnectsMu.Lock()
			assert.Equal(t, 2, reconnects)
			reconnectsMu.Unlock()
			socket.Disconnect()
			tw.Done()
		})
		socket.Connect()

		tw.WaitTimeout(t, DefaultConnectTimeout)
	})

	t.Run("should fire reconnect_* events on manager", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				ReconnectionAttempts: 2,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		close()
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/timeout_socket", nil)
		reconnects := 0
		reconnectsMu := sync.Mutex{}

		manager.OnReconnectAttempt(func(attempt uint32) {
			reconnectsMu.Lock()
			reconnects++
			reconnects := reconnects
			reconnectsMu.Unlock()
			assert.Equal(t, uint32(reconnects), attempt)
		})
		manager.OnReconnectFailed(func() {
			reconnectsMu.Lock()
			assert.Equal(t, 2, reconnects)
			reconnectsMu.Unlock()
			socket.Disconnect()
			tw.Done()
		})
		socket.Connect()

		tw.WaitTimeout(t, DefaultConnectTimeout)
	})

	t.Run("should not try to reconnect and should form a connection when connecting to correct port with default timeout", func(t *testing.T) {
		var (
			reconnectionDelay    = 10 * time.Millisecond
			reconnectionDelayMax = 10 * time.Millisecond
		)
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			&ManagerConfig{
				ReconnectionDelay:    &reconnectionDelay,
				ReconnectionDelayMax: &reconnectionDelayMax,
				EIO: eio.ClientConfig{
					Transports: []string{"polling"}, // To buy time by not waiting for +2 other transport's connection attempts.
				},
			},
		)
		tw := utils.NewTestWaiter(1)
		socket := manager.Socket("/valid", nil)

		manager.OnReconnectAttempt(func(attempt uint32) {
			socket.Disconnect()
			t.FailNow()
		})
		socket.OnConnect(func() {
			time.Sleep(1000 * time.Millisecond)
			socket.Disconnect()
			tw.Done()
		})
		socket.Connect()

		tw.WaitTimeout(t, DefaultConnectTimeout)
		close()
	})

	t.Run("should connect while disconnecting another socket", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			nil,
		)
		tw := utils.NewTestWaiter(1)
		socket1 := manager.Socket("/foo", nil)

		socket1.OnConnect(func() {
			socket2 := manager.Socket("/asd", nil)
			socket2.OnConnect(func() {
				tw.Done()
			})
			socket2.Connect()
			socket1.Disconnect()
		})
		socket1.Connect()

		tw.WaitTimeout(t, DefaultConnectTimeout)
		close()
	})

	t.Run("should not close the connection when disconnecting a single socket", func(t *testing.T) {
		_, _, manager, close := newTestServerAndClient(
			t,
			&ServerConfig{
				AcceptAnyNamespace: true,
			},
			nil,
		)
		tw := utils.NewTestWaiter(1)
		doneOnce := sync.OnceFunc(func() { tw.Done() })
		socket1 := manager.Socket("/foo", nil)
		socket2 := manager.Socket("/asd", nil)

		socket1.OnConnect(func() {
			socket2.Connect()
		})
		socket2.OnConnect(func() {
			socket2.OnDisconnect(func(reason Reason) {
				t.Fatal("should not happen for now")
			})
			socket1.Disconnect()
			time.Sleep(200 * time.Millisecond)
			socket2.OffDisconnect()
			manager.OnClose(func(reason Reason, err error) {
				doneOnce()
			})
			socket2.Disconnect()
		})
		socket1.Connect()

		tw.WaitTimeout(t, DefaultConnectTimeout)
		close()
	})

	t.Run("should receive ack", func(t *testing.T) {
		server, _, manager, close := newTestServerAndClient(t, nil, nil)
		socket := manager.Socket("/", nil)
		socket.Connect()
		tw := utils.NewTestWaiter(5)

		socket.OnConnect(func() {
			for i := 0; i < 5; i++ {
				t.Log("Emitting to server")
				socket.Emit("ack", "hello", func(reply string) {
					defer tw.Done()
					t.Logf("Ack received. Value: `%s`", reply)
					assert.Equal(t, "hi", reply)
				})
			}
		})

		server.OnConnection(func(socket ServerSocket) {
			socket.OnEvent("ack", func(message string, ack func(reply string)) {
				t.Logf("Message for the `ack` event: %s", message)
				assert.Equal(t, "hello", message)
				ack("hi")
			})
		})
		tw.WaitTimeout(t, utils.DefaultTestWaitTimeout)
		close()
	})
}
