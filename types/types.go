package types

type Paths struct {
	Txt  string `json:"txt,omitempty"`
	Json string `json:"json,omitempty"`
}

type Count struct {
	Txt  int `json:"txt,omitempty"`
	Json int `json:"json,omitempty"`
}

type VLoggoOutputs struct {
	Console bool `json:"console"`
	Text    bool `json:"text"`
	JSON    bool `json:"json"`
}

type VLoggoOutputsInput struct {
	Console *bool `json:"console,omitempty"`
	Text    *bool `json:"text,omitempty"`
	JSON    *bool `json:"json,omitempty"`
}

type VLoggoFormat struct {
	Timezone string `json:"timezone,omitempty"`
}

type VLoggoFormatInput struct {
	Timezone *string `json:"timezone,omitempty"`
}

// Size limits use bytes. Zero selects the documented default.
type VLoggoRotation struct {
	RetentionDays int   `json:"retentionDays,omitempty"`
	MaxFileSize   int64 `json:"maxFileSize,omitempty"`
	MaxTotalSize  int64 `json:"maxTotalSize,omitempty"`
}

type VLoggoRotationInput struct {
	RetentionDays *int   `json:"retentionDays,omitempty"`
	MaxFileSize   *int64 `json:"maxFileSize,omitempty"`
	MaxTotalSize  *int64 `json:"maxTotalSize,omitempty"`
}

type VLoggoSMTP struct {
	Host                 string   `json:"host,omitempty"`
	Port                 int      `json:"port,omitempty"`
	Username             string   `json:"username,omitempty"`
	Password             string   `json:"password,omitempty"`
	From                 string   `json:"from,omitempty"`
	To                   []string `json:"to,omitempty"`
	Secure               bool     `json:"secure,omitempty"`
	RequireTLS           bool     `json:"requireTLS,omitempty"`
	Debug                bool     `json:"debug,omitempty"`
	ConnectionTimeout    int      `json:"connectionTimeout,omitempty"`
	GreetingTimeout      int      `json:"greetingTimeout,omitempty"`
	SocketTimeout        int      `json:"socketTimeout,omitempty"`
	SecureSet            bool     `json:"-"`
	ConnectionTimeoutSet bool     `json:"-"`
	GreetingTimeoutSet   bool     `json:"-"`
	SocketTimeoutSet     bool     `json:"-"`
}

type VLoggoConfig struct {
	Client                 string         `json:"client,omitempty"`
	Json                   bool           `json:"json,omitempty"` // Legacy alias for Outputs.JSON.
	Notify                 bool           `json:"notify,omitempty"`
	Debug                  bool           `json:"debug,omitempty"`   // Legacy alias for Level.
	Console                bool           `json:"console,omitempty"` // Legacy alias for Outputs.Console.
	Throttle               int            `json:"throttle,omitempty"`
	Filecount              Count          `json:"filecount,omitempty"`
	Directory              Paths          `json:"directory,omitempty"`
	SMTP                   VLoggoSMTP     `json:"smtp,omitempty"`
	Outputs                VLoggoOutputs  `json:"outputs,omitempty"`
	Level                  LogLevel       `json:"level,omitempty"`
	Format                 VLoggoFormat   `json:"format,omitempty"`
	Rotation               VLoggoRotation `json:"rotation,omitempty"`
	Redact                 []string       `json:"redact,omitempty"`
	StartupMessage         bool           `json:"startupMessage,omitempty"`
	NotificationTimeout    int            `json:"notificationTimeout,omitempty"`
	RetryDelay             int            `json:"retryDelay,omitempty"`
	ClientSet              bool           `json:"-"`
	NotifySet              bool           `json:"-"`
	ConsoleSet             bool           `json:"-"`
	OutputsSet             bool           `json:"-"`
	LevelSet               bool           `json:"-"`
	SMTPSet                bool           `json:"-"`
	ThrottleSet            bool           `json:"-"`
	FilecountTxtSet        bool           `json:"-"`
	FilecountJsonSet       bool           `json:"-"`
	DirectoryTxtSet        bool           `json:"-"`
	DirectoryJsonSet       bool           `json:"-"`
	FormatTimezoneSet      bool           `json:"-"`
	RetentionDaysSet       bool           `json:"-"`
	MaxFileSizeSet         bool           `json:"-"`
	MaxTotalSizeSet        bool           `json:"-"`
	NotificationTimeoutSet bool           `json:"-"`
	RetryDelaySet          bool           `json:"-"`
	NotificationIssue      string         `json:"-"`
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
	Host              *string  `json:"host,omitempty"`
	Port              *int     `json:"port,omitempty"`
	Username          *string  `json:"username,omitempty"`
	Password          *string  `json:"password,omitempty"`
	From              *string  `json:"from,omitempty"`
	To                []string `json:"to,omitempty"`
	Secure            *bool    `json:"secure,omitempty"`
	RequireTLS        *bool    `json:"requireTLS,omitempty"`
	Debug             *bool    `json:"debug,omitempty"`
	ConnectionTimeout *int     `json:"connectionTimeout,omitempty"`
	GreetingTimeout   *int     `json:"greetingTimeout,omitempty"`
	SocketTimeout     *int     `json:"socketTimeout,omitempty"`
}

type VLoggoConfigInput struct {
	Client              *string              `json:"client,omitempty"`
	Json                *bool                `json:"json,omitempty"`
	Notify              *bool                `json:"notify,omitempty"`
	Debug               *bool                `json:"debug,omitempty"`
	Console             *bool                `json:"console,omitempty"`
	Throttle            *int                 `json:"throttle,omitempty"`
	Filecount           *CountInput          `json:"filecount,omitempty"`
	Directory           *PathsInput          `json:"directory,omitempty"`
	SMTP                *VLoggoSMTPInput     `json:"smtp,omitempty"`
	Outputs             *VLoggoOutputsInput  `json:"outputs,omitempty"`
	Level               *LogLevel            `json:"level,omitempty"`
	Format              *VLoggoFormatInput   `json:"format,omitempty"`
	Rotation            *VLoggoRotationInput `json:"rotation,omitempty"`
	Redact              *[]string            `json:"redact,omitempty"`
	StartupMessage      *bool                `json:"startupMessage,omitempty"`
	NotificationTimeout *int                 `json:"notificationTimeout,omitempty"`
	RetryDelay          *int                 `json:"retryDelay,omitempty"`
}

type LogLevel string

const (
	Debug LogLevel = "DEBUG"
	Info  LogLevel = "INFO"
	Warn  LogLevel = "WARN"
	Error LogLevel = "ERROR"
	Fatal LogLevel = "FATAL"
)

type LogData map[string]any
type LogError map[string]any

type LogEntry struct {
	EventID   string   `json:"eventId"`
	Client    string   `json:"client"`
	Level     LogLevel `json:"level"`
	Timestamp string   `json:"timestamp"`
	Code      string   `json:"code"`
	Caller    string   `json:"caller"`
	Message   string   `json:"message"`
	Context   LogData  `json:"context,omitempty"`
	Data      LogData  `json:"data,omitempty"`
	Error     LogError `json:"error,omitempty"`
}
