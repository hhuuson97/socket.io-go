package eio

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/madflojo/testcerts"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tomruk/socket.io-go/engine.io/parser"
	"nhooyr.io/websocket"
)

func TestWebTransport(t *testing.T) {
	t.Run("should allow to connect with WebTransport directly", func(t *testing.T) {
		var (
			tw          = NewTestWaiter(2)
			checkIndex  = 0
			testPackets = []*parser.Packet{
				mustCreatePacket(t, parser.PacketTypeMessage, false, []byte("12345678")),
				mustCreatePacket(t, parser.PacketTypeMessage, false, []byte("87654321")),
			}
		)

		onSocket := func(socket ServerSocket) *Callbacks {
			return &Callbacks{
				OnPacket: func(packets ...*parser.Packet) {
					for _, packet := range packets {
						if packet.Type != parser.PacketTypeMessage {
							continue
						}
						defer tw.Done()
						testPacket := testPackets[checkIndex]
						assert.Equal(t, testPacket.IsBinary, packet.IsBinary)
						assert.Equal(t, testPacket.Data, packet.Data)
						checkIndex++
					}
				},
			}
		}
		_, _, ts, close := newWebTransportTestServer(t, onSocket, nil, nil)

		clientConfig := &ClientConfig{
			UpgradeDone: func(transportName string) {
				t.Fail() // Connection should be directly via WebTransport.
			},
			HTTPTransport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
			WebSocketDialOptions: &websocket.DialOptions{
				HTTPClient: &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				},
			},
			WebTransportDialer: &webtransport.Dialer{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		clientConfig.Transports = []string{"webtransport"}
		socket := testDial(t, ts.URL, nil, clientConfig, nil)
		socket.Send(testPackets...)

		require.Equal(t, "webtransport", socket.TransportName())

		tw.WaitTimeout(t, DefaultTestWaitTimeout)
		close()
	})
}

func newWebTransportTestServer(
	t *testing.T,
	onSocket NewSocketCallback,
	config *ServerConfig,
	options *testServerOptions,
) (io *Server, wtServer *webtransport.Server, ts *httptest.Server, close func()) {
	if config == nil {
		config = new(ServerConfig)
	}
	if options == nil {
		options = new(testServerOptions)
	}
	enablePrintDebugger := os.Getenv("EIO_DEBUGGER_PRINT") == "1"
	if enablePrintDebugger {
		config.Debugger = NewPrintDebugger()
	}

	certFile, keyFile, err := testcerts.GenerateCertsToTempFile(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	wtServer = &webtransport.Server{}
	config.WebTransportServer = wtServer
	io = newServer(onSocket, config, options.testWaitUpgrade)
	err = io.Run()
	if err != nil {
		t.Fatal(err)
	}

	ts = httptest.NewUnstartedServer(io)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	ts.StartTLS()

	wtServer.H3.Addr = ts.Listener.Addr().String()
	wtServer.H3.Handler = io
	go func() {
		err := wtServer.ListenAndServeTLS(certFile, keyFile)
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	close = func() {
		os.Remove(certFile)
		os.Remove(keyFile)
		err = io.Close()
		if err != nil {
			t.Fatalf("io.Close: %s", err)
		}
		err = wtServer.Close()
		if err != nil {
			t.Fatalf("io.Close: %s", err)
		}
		ts.Close()
	}
	return
}
