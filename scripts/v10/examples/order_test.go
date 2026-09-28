package cancel

import (
	"strings"
	"testing"
)

func TestCanCancel(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "pending", want: true},
		{status: "paid", want: true},
		{status: "shipped", want: false},
	}
	for _, example := range cases {
		got := CanCancel(example.status)
		if got != example.want {
			t.Errorf("cancellation mismatch: status=%s got=%t want=%t", example.status, got, example.want)
		}
	}
	t.Log(strings.Repeat("alpha-capture-detail-", 700))
}
