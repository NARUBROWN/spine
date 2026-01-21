package boot

import "time"

type Options struct {
	Address                string
	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
}
