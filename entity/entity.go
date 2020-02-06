package entity

// The json format for exporter messages
// {
// 	"dtype": "data_metric",
// 	"action": "insert",
// 	"userid": "xxxx", // login id - from collector
// 	"source": "xxxx", // uuid
// 	"path": "xxxx",
// 	"time": "xxxx",
// 	"timestamp": "xxxx", // from collector
// 	"data": {
// 		"xxx": "xxxx"
// 	}
// }

// Message structure
type Message struct {
	Kind      string                 `json:"dtype"`
	Action    string                 `json:"action"`
	UserID    int                    `json:"userid"`
	Source    string                 `json:"source,omitempty"`
	Path      string                 `json:"path,omitempty"`
	Time      string                 `json:"time,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// The yaml format in consul
// fixed_columns: ["userid", "source", "path", "timestamp"]
// relations:
//   - dtype: "data_heartbeat"
//     table: "heartbeat"
//     columns: ["status"]

// The json format
// {
//     "dtype": "data_heartbeat",
//     "action": "insert",
//     "userid": "xxxx",
//     "source": "xxxx",
//     "path": "xxxx",
//     "timestamp": "xxxx",
//     "data": {
//         "status": "OK"
//     }
// }

// HeartbeatData structure
type HeartbeatData struct {
	Status string `json:"status"`
}

// Heartbeat structure
type Heartbeat struct {
	Kind      string        `json:"dtype"`
	Action    string        `json:"action"`
	UserID    int           `json:"userid"`
	Source    string        `json:"source"`
	Path      string        `json:"path"`
	Data      HeartbeatData `json:"data"`
	Timestamp string        `json:"timestamp"`
}
