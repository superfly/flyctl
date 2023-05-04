package flyproxy

type Destination struct {
	Proto string `json:"proto"`
	Port  uint16 `json:"port"`
}
