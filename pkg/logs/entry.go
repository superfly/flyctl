package logs

type LogEntry struct {
	Level     string `json:"level"`
	Instance  string `json:"instance"`
	Message   string `json:"message"`
	Region    string `json:"region"`
	Timestamp string `json:"timestamp"`
	Meta      Meta   `json:"meta"`
}

type Meta struct {
	Instance string
	Region   string
	Event    struct {
		Provider string
	}
	HTTP struct {
		Request struct {
			ID      string
			Method  string
			Version string
		}
		Response struct {
			StatusCode int `json:"status_code"`
		}
	}
	Error struct {
		Code    int
		Message string
	}
	URL struct {
		Full string
	}
}

type natsLog struct {
	Event struct {
		Provider string `json:"provider"`
	} `json:"event"`
	Fly struct {
		App struct {
			Instance string `json:"instance"`
			Name     string `json:"name"`
		} `json:"app"`
		Region string `json:"region"`
	} `json:"fly"`
	Host string `json:"host"`
	Log  struct {
		Level string `json:"level"`
	} `json:"log"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}
