package models

// hvd
type Instance struct {
	Name          string                      `json:"name"`
	LiveRestreams map[string]LiveRestreamResp `json:"restreams"`
}
