package requests

import (
	//"fmt"
	"net/http"
	"testing"
	"bytes"
	//"errors"
	"io"
	"net/url"
	"io/ioutil"
	"os"
	"reflect"
	"time"
	//"runtime"
)

// Stub `requests` namespace
type stubRequests struct {
	Get      func(string, string, map[string]string) (*stubResponse, error)
	GetAsync func(string, string, map[string]string, int) (chan interface{}, error)
	//Post     func(string, string, map[string]interface{}) (*http.Response, error)
}

// Stub connection
type stubConnection struct {
	channel chan interface{}
	done chan bool
	latency time.Duration
}

func newStubConnection(latency time.Duration) *stubConnection {
	channel := make(chan interface{}, 1)
	done := make(chan bool, 1)
	connection := &stubConnection{
		channel: channel,
		done: done,
		latency: latency,
	}
	return connection
}

func (c *stubConnection) Close() {
	close(c.channel)
	close(c.done)
}

// Stub `http.Request`
type stubRequest struct { http.Request }

func newStubRequest(method, rawurl string, body io.ReadCloser) (*stubRequest, error) {
	uri, err := url.ParseRequestURI(rawurl)
	if err != nil {
		panic("Something's wrong with your URI")
	}
	request := &stubRequest{
		Request: http.Request{
			Method: method,
			URL: uri,
			Body: body,
		},
	}
	return request, nil
}

// Stub `http.Response`
type stubResponse struct { http.Response }

func newStubResponse(status string, code int, header http.Header, body io.ReadCloser) *stubResponse {
	response := &stubResponse{
		Response: http.Response{
			Status: status,
			StatusCode: code,
			Proto: "HTTP/1.0",
			Header: header,
			Body: body,
		},
	}
	return response
}

// Stub `http.Server`
type stubServer struct {
	http.Server
	response *stubResponse
	latency  time.Duration
}

func newStubServer(addr string, res *stubResponse, lat time.Duration) *stubServer {
	server := &stubServer{
		Server: http.Server{ Addr: addr },
		response: res,
		latency: lat,
	}
	return server
}

func (s *stubServer) Reply(code statusCode) *stubResponse {
	// Block for server's latency
	//<-time.Tick(s.latency)
	time.Sleep(s.latency)
	// Create status code and return the response
	s.response.StatusCode = (int)(code)
	// TODO: Assign Status
	return s.response
}

// Stub the `http.Client`
type stubClient struct { http.Client }

func newStubClient(timeout time.Duration) *stubClient {
	client := &stubClient{
		Client: http.Client{ Timeout: timeout },
	}
	return client
}

// Inject `*stubConnection` and `*stubServer` to simulate a server call
func (c *stubClient) Do(req *stubRequest, conn *stubConnection, server *stubServer) (*stubResponse, error) {
	// Block for the duration of `conn.latency` + `server.latency`
	// to simulaate real-world latencies and test timeoutn
	code := (statusCode)(server.response.StatusCode)
	conn.channel <- req
	go func() {
		//request := <-conn
		//<-time.Tick(conn.latency)
		time.Sleep(conn.latency)
		<-conn.channel
		// TODO: Do something with the receive request
		conn.channel <- server.Reply(code)
		//<-time.Tick(server.latency)
		time.Sleep(server.latency)
		conn.done <- true
	}()
		
	<-conn.done
	res := <-conn.channel
	return res.(*stubResponse), nil
}

func (c *stubClient) DoAsync(req *stubRequest, conn *stubConnection, server *stubServer) (chan interface{}, error) {
	code := (statusCode)(server.response.StatusCode)

	// Network latency
	time.Sleep(conn.latency)
	conn.channel <- req

	go func(conn *stubConnection) {
		<-conn.channel
		// TODO: Do something with the receive request
		conn.channel <- server.Reply(code)
		//<-time.Tick(server.latency)
	}(conn)
	return conn.channel, nil
}

var (
	// Setup connection
	networkLatency = time.Duration(100) * time.Millisecond
	conn = newStubConnection(networkLatency)

	// Setup client
	timeout = time.Duration(3) * time.Second
	client = newStubClient(timeout)

	// Setup server
	res = newStubResponse("200 OK", 200, header, body)
	endpoint = "http://jochasinga.io"
	serverLatency = time.Duration(100) * time.Millisecond
	server = newStubServer(endpoint, res, serverLatency)

	// Setup request
	header = http.Header{}
	jsonStr = `{"foo": ["bar", "baz"]}`
	body = ioutil.NopCloser(bytes.NewBuffer([]byte(jsonStr)))
	auth = map[string]string{"user": "pass"}
	requests = &stubRequests{
		Get: func(url, body string, auth map[string]string) (*stubResponse, error) {
			// Convert body from string to io.ReadCloser
			bodyReadCloser := ioutil.NopCloser(bytes.NewBuffer([]byte(body)))
			//req, err := http.NewRequest("GET", url, bodyReadCloser)
			req, err := newStubRequest("GET", url, bodyReadCloser)
			if err != nil {
				panic(err)
			}
			// TODO: include basic auth
			/*
			if len(auth) > 0 {
				for user, password := range auth {
					req.SetBasicAuth(user, password)
				}
			}
                        */
			res, err := client.Do(req, conn, server)
			if err != nil {
				panic(err)
			}
			return res, nil
		},
		GetAsync: func(url, body string, auth map[string]string, timeout int) (chan interface{}, error) {
			waitUntil := time.Duration(timeout) * time.Second
			data := ioutil.NopCloser(bytes.NewBuffer([]byte(body)))
			client.Timeout = waitUntil
			
			req, err := newStubRequest("GET", url, data)
			if err != nil {
				panic(err)
			}
			// TODO: include basic auth
			/*
			if len(auth) > 0 {
				for user, password := range auth {
					req.SetBasicAuth(user, password)
				}
			}
                        */
			channel, err := client.DoAsync(req, conn, server)
			if err != nil {
				panic(err)
			}
			
			return channel, nil
			

			/*
			p := NewPromise(func() (*http.Response, error) {
				re := make(chan *http.Response)
				er := make(chan error)
				go func() {
					res, err := client.Do(req, server)
					if err != nil {
						er <- err
					}
					re <- res
					return
				}()
				defer close(re)
				defer close(er)
				return (*http.Response)(nil), errors.New("Time out")
			})
			return p, nil
                        */
		},
	}
)

// Get response back as `*stubResponse`
func TestGetResponseType(t *testing.T) {
	resp, err := requests.Get(endpoint, jsonStr, auth)
	if err != nil {
		t.Error(err)
	}
	returnType := reflect.TypeOf(resp)
	responseType := reflect.TypeOf((*stubResponse)(nil))
	if returnType != responseType {
		t.Errorf("Expected return type of `*stubResponse`, but it was %v instead.", returnType)
	}
}

// Get response back with status 200
func TestGetResponseStatus(t *testing.T) {
	resp, err := requests.Get(endpoint, jsonStr, auth)
	if err != nil {
		t.Error(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected StatusCode `200`, but it was %s instead.", resp.Status)
	}
}

func TestGetAsyncResponseType(t *testing.T) {
	// 3 seconds timeout
	timeout := 1
	resultChan, err := requests.GetAsync(endpoint, jsonStr, auth, timeout)
	if err != nil {
		t.Error(err)
	}
	returnType := reflect.TypeOf(resultChan)
	responseType := reflect.TypeOf(chan interface{}(nil))
	if returnType != responseType {
		t.Errorf("Expected return type of `chan interface{}`, but it was %v instead.", returnType)
	}
}

/*
func TestGetAsyncResponseStatus(t *testing.T) {
	timeout := 0
	p, err := requests.GetAsync("http://example.com", htmlStr, auth, timeout)
	if err != nil {
		t.Error(err)
	}

	result := p.Then(func() *http.Response { return p.res }, func() error { return p.err })

	// Result should be either `*http.Response` or `error`
	switch result := result.(type) {
	default:
		t.Error()
	case *http.Response:
    		if result.Status != "200 OK" {
			t.Errorf("Expected Status `200 OK`, but it was %s instead.", result.Status)
		}
		break
	case error:
		if result.Error() == "" {
			t.Errorf("Expected `error.`")
		}
		break
	}
}
*/

func TestMain(m *testing.M) {
	v := m.Run()
	defer conn.Close()
	if v == 0 {
		os.Exit(1)
	}
	os.Exit(v)
}
