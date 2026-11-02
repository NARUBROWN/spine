package application

import "testing"

func TestBuildApplication(t *testing.T) {
	for _, tt := range []struct {
		name    string
		port    string
		address string
		want    string
	}{
		{name: "default", want: "127.0.0.1:8080"},
		{name: "CLI_port", port: "9123", want: "127.0.0.1:9123"},
		{name: "explicit_address", port: "9123", address: "127.0.0.1:0", want: "127.0.0.1:0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SPINE_PORT", tt.port)
			t.Setenv("SPINE_DEMO_ADDRESS", tt.address)
			app, options, err := BuildApplication()
			if err != nil {
				t.Fatal(err)
			}
			if err := app.Validate(options); err != nil {
				t.Fatalf("최소 앱은 네트워크 없이 사전 검증되어야 합니다: %v", err)
			}
			if options.Address != tt.want || options.HTTP == nil || !options.EnableGracefulShutdown {
				t.Fatalf("앱 실행 설정이 다릅니다: %+v", options)
			}
			if options.Kafka != nil || options.RabbitMQ != nil || len(app.Consumers().Registrations()) != 0 {
				t.Fatal("최소 E2E 앱은 외부 브로커를 요구하지 않아야 합니다")
			}
		})
	}
}
