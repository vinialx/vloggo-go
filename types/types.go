package types

type Paths struct {
	Txt  string
	Json string
}

type Count struct {
	Txt  int
	Json int
}

type SMTP struct {
	Server   string   `env:"SMTP_HOST"`
	Port     int      `env:"SMTP_PORT"`
	Username string   `env:"SMTP_USERNAME"`
	Password string   `env:"SMTP_PASSWORD"`
	From     string   `env:"SMTP_FROM"`
	To       []string `env:"SMTP_TO"`
}

type VLoggo struct {
	Client    string
	Json      bool
	Debug     bool
	Notify    bool
	Console   bool
	Throttle  int
	Filecount Count
	Directory Paths
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
	Code    int      `json:"code"`
	Caller  string   `json:"caller"`
	Message string   `json:"message"`
}
