package auth

import "testing"

func TestWeakPasswordError_Description(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		violations []string
		want       string
	}{
		{
			name:       "no violations",
			violations: nil,
			want:       "A senha informada é inválida.",
		},
		{
			name:       "one violation",
			violations: []string{"no mínimo 8 caracteres"},
			want:       "A senha é muito fraca. Ela deve ter no mínimo 8 caracteres.",
		},
		{
			name:       "two violations",
			violations: []string{"no mínimo 8 caracteres", "uma letra maiúscula"},
			want:       "A senha é muito fraca. Ela deve ter no mínimo 8 caracteres e uma letra maiúscula.",
		},
		{
			name:       "three violations",
			violations: []string{"no mínimo 8 caracteres", "uma letra maiúscula", "um número"},
			want:       "A senha é muito fraca. Ela deve ter no mínimo 8 caracteres, uma letra maiúscula e um número.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := &WeakPasswordError{Violations: tc.violations}
			if got := err.Description(); got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
