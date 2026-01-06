package graph

import "time"

// Add your custom model types here
// These will be used by the generated resolvers

// Example model - replace with your own
type User struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}
