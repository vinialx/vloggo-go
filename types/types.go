package types

type Paths struct {
	Txt  string `json:"txt,omitempty"`
	Json string `json:"json,omitempty"`
}

type Count struct {
	Txt  int `json:"txt,omitempty"`
	Json int `json:"json,omitempty"`
}

type VLoggoSMTP struct {
	Host     string   `json:"host,omitempty"`
	Port     int      `json:"port,omitempty"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	From     string   `json:"from,omitempty"`
	To       []string `json:"to,omitempty"`
	Secure   bool     `json:"secure,omitempty"`
}

type VLoggoConfig struct {
	Client    string     `json:"client,omitempty"`
	Json      bool       `json:"json,omitempty"`
	Notify    bool       `json:"notify,omitempty"`
	Debug     bool       `json:"debug,omitempty"`
	Console   bool       `json:"console,omitempty"`
	Throttle  int        `json:"throttle,omitempty"`
	Filecount Count      `json:"filecount,omitempty"`
	Directory Paths      `json:"directory,omitempty"`
	SMTP      VLoggoSMTP `json:"smtp,omitempty"`
}

type PathsInput struct {
	Txt  *string `json:"txt,omitempty"`
	Json *string `json:"json,omitempty"`
}

type CountInput struct {
	Txt  *int `json:"txt,omitempty"`
	Json *int `json:"json,omitempty"`
}

type VLoggoSMTPInput struct {
	Host     *string  `json:"host,omitempty"`
	Port     *int     `json:"port,omitempty"`
	Username *string  `json:"username,omitempty"`
	Password *string  `json:"password,omitempty"`
	From     *string  `json:"from,omitempty"`
	To       []string `json:"to,omitempty"`
	Secure   *bool    `json:"secure,omitempty"`
}

type VLoggoConfigInput struct {
	Client    *string          `json:"client,omitempty"`
	Json      *bool            `json:"json,omitempty"`
	Notify    *bool            `json:"notify,omitempty"`
	Debug     *bool            `json:"debug,omitempty"`
	Console   *bool            `json:"console,omitempty"`
	Throttle  *int             `json:"throttle,omitempty"`
	Filecount *CountInput      `json:"filecount,omitempty"`
	Directory *PathsInput      `json:"directory,omitempty"`
	SMTP      *VLoggoSMTPInput `json:"smtp,omitempty"`
}

type LogLevel string

const (
	Info  LogLevel = "INFO"
	Warn  LogLevel = "WARN"
	Error LogLevel = "ERROR"
	Fatal LogLevel = "FATAL"
	Debug LogLevel = "DEBUG"
)

type LogEntry struct {
	Level   LogLevel `json:"level"`
	Code    string   `json:"code"`
	Caller  string   `json:"caller"`
	Message string   `json:"message"`
}
