//Package stratum implements the basic stratum protocol.
// This is normal jsonrpc but the go standard library is insufficient since we need features like notifications.
package stratum

import "encoding/json"

// Request : A remote method is invoked by sending a request to the remote stratum service.
type Request struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     uint64   `json:"id"`
}

// Response is the stratum server's response on a Request
type Response struct {
	ID     uint64           `json:"id"`
	Result *json.RawMessage `json:"result"`
	Error  error            `json:"error,string"`
}

// Notification is a special kind of Request, it has no ID and is sent from the server to the client
type Notification struct {
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params"`
}
