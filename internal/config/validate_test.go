package config

import (
	"testing"

	"github.com/alebeck/boring/internal/tunnel"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		tunnels []tunnel.Desc
		wantErr bool
	}{
		{"valid", []tunnel.Desc{
			{Name: "dev", Group: "work"},
			{Name: "prod"},
		}, false},
		{"duplicate name", []tunnel.Desc{{Name: "dev"}, {Name: "dev"}}, true},
		{"empty name", []tunnel.Desc{{Name: ""}}, true},
		{"spaced name", []tunnel.Desc{{Name: "my tunnel"}}, true},
		{"glob name", []tunnel.Desc{{Name: "dev*"}}, true},
		{"bad group", []tunnel.Desc{{Name: "dev", Group: "my group"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.tunnels)
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
