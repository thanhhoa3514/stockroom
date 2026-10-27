package cli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		command string
	}{{[]string{"--db", "test.db", "start", "--scenario", "outage", "--fail-batch", "3"}, "start"}, {[]string{"resume", "--scenario", "outage"}, "resume"}, {[]string{"status", "--run", "2"}, "status"}, {[]string{"products", "--status", "withdrawn"}, "products"}} {
		c, err := Parse(tt.args, io.Discard)
		if err != nil || c.Command != tt.command {
			t.Fatal(c, err)
		}
	}
	for _, args := range [][]string{nil, {"unknown"}, {"status", "--run", "-1"}, {"products", "--status", "bad"}, {"start", "extra"}, {"start", "--fail-batch", "-1"}} {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	var out bytes.Buffer
	if _, err := Parse([]string{"--help"}, &out); !errors.Is(err, flag.ErrHelp) || out.Len() == 0 {
		t.Fatal(err, out.String())
	}
}
