package cmd

// type Bedrock struct {
// 	Run RunCmd `cmd:"" help:"Run the server."`
// }

type Cli struct {
	DatabaseUrl     string `env:"DATABASE_URL" help:"the url of the Postgres database"`
	Port            int    `env:"PORT" default:"3000" help:"the port to run on"`
	Debug           bool   `env:"DEBUG" help:"Enable debug mode."`
	Dev             bool   `env:"DEV" help:"Enable dev mode (for CORS)."`
	DisableJSONLogs bool   `env:"DISABLE_JSON_LOGS" help:"disable json loggin"`
	CachePath       string `env:"CACHE_PATH" type:"path" default:"./data"`
}
