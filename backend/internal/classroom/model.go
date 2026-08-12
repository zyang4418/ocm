package classroom

import "time"

// Classroom is a bookable teaching space. It is the foundation entity that
// future booking and repair modules will reference via classroom ID.
//
// Floor/Campus accommodate the 教务处 classroom export (楼层/校区), which the
// aggregated schedule import splits out alongside the core fields.
type Classroom struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Building    string    `json:"building"`
	Capacity    int       `json:"capacity"`
	Type        string    `json:"type"`
	Floor       string    `json:"floor"`
	Campus      string    `json:"campus"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ClassroomInput carries the mutable fields shared by create and update.
type ClassroomInput struct {
	Name        string `json:"name"`
	Building    string `json:"building"`
	Capacity    int    `json:"capacity"`
	Type        string `json:"type"`
	Floor       string `json:"floor"`
	Campus      string `json:"campus"`
	Status      string `json:"status"`
	Description string `json:"description"`
}
