package omni

import "context"

// Perspective is a built-in (nil ID) or custom OmniFocus perspective.
type Perspective struct {
	ID      *string `json:"id"`
	Name    string  `json:"name"`
	Builtin bool    `json:"builtin"`
}

// Perspectives lists built-in and custom perspectives.
func (b *Bridge) Perspectives(ctx context.Context) ([]Perspective, error) {
	var out []Perspective
	if err := b.Call(ctx, "list_perspectives", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
