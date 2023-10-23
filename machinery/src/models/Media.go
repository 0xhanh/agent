package models

type Media struct {
	Key        string `json:"key"`
	Path       string `json:"path"`
	Day        string `json:"day"`
	ShortDay   string `json:"short_day"`
	Time       string `json:"time"`
	Timestamp  string `json:"timestamp"`
	CameraName string `json:"camera_name"`
	CameraKey  string `json:"camera_key"`
}

type EventFilter struct {
	TimestampOffsetStart int64 `json:"timestamp_offset_start"`
	TimestampOffsetEnd   int64 `json:"timestamp_offset_end"`
	NumberOfElements     int   `json:"number_of_elements"`
}

// hvd
type LiveRestreamReq struct {
	ReqId         string `json:"req_id,omitempty" bson:"req_id"`
	ReqTime       int64  `json:"req_time,omitempty" bson:"req_time"`
	ReqType       string `json:"req_type" bson:"req_type"`
	Stream        string `json:"stream" bson:"stream"`
	ProxyAddr     string `json:"proxy_addr" bson:"proxy_addr"`
	ProxyUsername string `json:"proxy_username,omitempty" bson:"proxy_username"`
	ProxyPassword string `json:"proxy_password,omitempty" bson:"proxy_password"`
}

// hvd
type LiveRestreamResp struct {
	RespId   string `json:"resp_id,omitempty" bson:"resp_id"`
	ReqId    string `json:"req_id,omitempty" bson:"req_id"`
	RespTime int64  `json:"resp_time" bson:"resp_time"`
	Duration int64  `json:"duration,omitempty" bson:"duration"`
	Stream   string `json:"stream" bson:"stream"`
	Result   string `json:"result,omitempty" bson:"result"`
	Status   string `json:"status,omitempty" bson:"status"`
}
