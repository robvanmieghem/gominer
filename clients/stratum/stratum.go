//Package stratum implements the basic stratum protocol.
// This is normal jsonrpc but the go standard library is insufficient since we need features like notifications.
package stratum

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
)

// request : A remote method is invoked by sending a request to the remote stratum service.
type request struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     uint64   `json:"id"`
}

// response is the stratum server's response on a Request
// notification is an inline struct to easily decode messages in a response/notification using a json marshaller
type response struct {
	ID           uint64           `json:"id"`
	Result       *json.RawMessage `json:"result"`
	Error        error            `json:"error,string"`
	notification `json:",inline"`
}

// notification is a special kind of Request, it has no ID and is sent from the server to the client
type notification struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

//ErrorCallback is the type of function that be registered to be notified of errors requiring a client
// to be dropped and a new one to be created
type ErrorCallback func(err error)

//NotificationHandler is the signature for a function that handles notifications
type NotificationHandler func(args []interface{})

// Client maintains a connection to the stratum server and (de)serializes requests/reponses/notifications
type Client struct {
	socket               net.Conn
	seqmutex             sync.Mutex // protects following
	seq                  uint64
	ErrorCallback        ErrorCallback
	notificationHandlers map[string]NotificationHandler
}

//Dial connects to a stratum+tcp at the specified network address.
// This function is not threadsafe
// If an error occurs, it is both returned here and through the ErrorCallback of the Client
func (c *Client) Dial(host string) (err error) {
	c.socket, err = net.Dial("tcp", host)
	if err != nil {
		c.dispatchError(err)
		return
	}
	go c.Listen()
	return
}

//Close releases the tcp connection
func (c *Client) Close() {
	if c.socket != nil {
		c.socket.Close()
	}
}

//SetNotificationHandler registers a function to handle notification for a specific method.
// This function is not threadsafe and all notificationhandlers should be set prior to calling the Dial function
func (c *Client) SetNotificationHandler(method string, handler NotificationHandler) {
	if c.notificationHandlers == nil {
		c.notificationHandlers = make(map[string]NotificationHandler)
	}
	c.notificationHandlers[method] = handler
}

func (c *Client) dispatchNotification(n notification) {
	if c.notificationHandlers == nil {
		return
	}
	if notificationHandler, exists := c.notificationHandlers[n.Method]; exists {
		notificationHandler(n.Params)
	}
}

func (c *Client) dispatch(r response) {
	if r.ID == 0 {
		c.dispatchNotification(r.notification)
		return
	}
	//TODO: dispatch normal response
}

func (c *Client) dispatchError(err error) {
	if c.ErrorCallback != nil {
		c.ErrorCallback(err)
	}
}

//Listen reads data from the open connection, deserializes it and dispatches the reponses and notifications
// This is a blocking function and will continue to listen until an error occurs (io or deserialization)
func (c *Client) Listen() {
	reader := bufio.NewReader(c.socket)
	for {
		rawmessage, err := reader.ReadString('\n')
		if err != nil {
			c.dispatchError(err)
			return
		}
		r := response{}
		err = json.Unmarshal([]byte(rawmessage), &r)
		if err != nil {
			c.dispatchError(err)
			return
		}
		c.dispatch(r)
	}
}

//Call invokes the named function, waits for it to complete, and returns its error status.
func (c *Client) Call(serviceMethod string, args []string, reply interface{}) (err error) {
	r := request{Method: serviceMethod, Params: args}

	c.seqmutex.Lock()
	c.seq++
	r.ID = c.seq
	c.seqmutex.Unlock()

	rawmsg, err := json.Marshal(r)
	if err != nil {
		return
	}
	//TODO: call, err := c.registerRequest(r)

	_, err = c.socket.Write(rawmsg)
	if err != nil {
		//TODO: c.cancelRequest(r.ID)
		return
	}
	_, err = c.socket.Write([]byte("\n"))
	if err != nil {
		//TODO: c.cancelRequest(r.ID)
		return
	}
	//TODO:
	// call.SetTimeout(10, func() {
	// 	c.cancelRequest(r.ID)
	// })
	// return call.Wait()
	return
}
