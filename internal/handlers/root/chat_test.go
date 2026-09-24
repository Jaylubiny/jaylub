package handlers

import "testing"

func TestValidateChatMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "trims whitespace", input: "  hello  ", want: "hello"},
		{name: "rejects empty", input: " \n\t ", wantErr: true},
		{name: "rejects long message", input: string(make([]rune, chatMaxMessageLength+1)), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateChatMessage(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateChatMessage() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("validateChatMessage() = %q, want %q", got, test.want)
			}
		})
	}
}
