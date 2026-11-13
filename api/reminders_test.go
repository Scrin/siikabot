package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Scrin/siikabot/commands/remind"
)

func TestDeleteReminderFailure(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{remind.ErrNotFound, http.StatusNotFound},
		{remind.ErrAlreadyDelivered, http.StatusConflict},
		{fmt.Errorf("wrapped: %w", remind.ErrAlreadyDelivered), http.StatusConflict},
		{errors.New("connection refused"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		if got, _ := deleteReminderFailure(tt.err); got != tt.want {
			t.Errorf("deleteReminderFailure(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}
