package loki

import "encoding/json"

type QueryRangeParams struct {
	Query     string
	Start     string
	End       string
	Limit     int
	Direction string
}

type QueryParams struct {
	Query     string
	Limit     int
	Time      string
	Direction string
}

type LabelsParams struct {
	Start string
	End   string
}

type SeriesParams struct {
	Match []string
	Start string
	End   string
}

type QueryResponse struct {
	Status string    `json:"status"`
	Data   QueryData `json:"data"`
}

func (r *QueryResponse) GetStatus() string { return r.Status }

type QueryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
	Stats      json.RawMessage `json:"stats"`
}

type StreamResult struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type MatrixResult struct {
	Metric map[string]string `json:"metric"`
	Values [][]any            `json:"values"`
}

type LabelsResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

func (r *LabelsResponse) GetStatus() string { return r.Status }

type SeriesResponse struct {
	Status string              `json:"status"`
	Data   []map[string]string `json:"data"`
}

func (r *SeriesResponse) GetStatus() string { return r.Status }

type lokiErrorResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}
