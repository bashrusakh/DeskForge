package admin

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestChangeCurPasswordFormValidation(t *testing.T) {
	validate := validator.New()
	tests := []struct {
		name  string
		form  ChangeCurPasswordForm
		valid bool
	}{
		{
			name: "accepts legacy old password longer than new password limit",
			form: ChangeCurPasswordForm{
				OldPassword: strings.Repeat("o", 33),
				NewPassword: strings.Repeat("n", 8),
			},
			valid: true,
		},
		{
			name: "keeps new password maximum length",
			form: ChangeCurPasswordForm{
				OldPassword: strings.Repeat("o", 33),
				NewPassword: strings.Repeat("n", 33),
			},
			valid: false,
		},
		{
			name: "keeps old password minimum length",
			form: ChangeCurPasswordForm{
				OldPassword: strings.Repeat("o", 3),
				NewPassword: strings.Repeat("n", 8),
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validate.Struct(tt.form) == nil; got != tt.valid {
				t.Fatalf("validation result = %t, want %t", got, tt.valid)
			}
		})
	}
}
