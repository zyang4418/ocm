package user

import "time"

// AdminClass is an administrative class (行政班): a persistent cohort of
// students identified by grade + name, e.g. grade="2024级", name="计算机244".
// Admin classes live in the user/people module because they are an
// organizational unit, not a course-delivery concept.
type AdminClass struct {
	ID        int64     `json:"id"`
	Grade     string    `json:"grade"`
	Name      string    `json:"name"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

// AdminClassInput is the create/update payload for an admin class.
type AdminClassInput struct {
	Grade string `json:"grade"`
	Name  string `json:"name"`
	Note  string `json:"note"`
}

// TeachingClass is a 教学班: a named group of admin classes that are taught
// together (合班). An offering is taught to exactly one teaching class, so the
// teaching class -- not the admin class -- is what distinguishes two offerings
// of the same course/teacher/semester taught to different groups.
type TeachingClass struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

// ClassRef is a lightweight admin-class reference (id + name) used inside
// teaching-class views.
type ClassRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Grade is included so the UI can render the full "2024级/计算机244" label.
	Grade string `json:"grade"`
}

// TeachingClassView is a teaching class joined with its member admin classes.
type TeachingClassView struct {
	TeachingClass
	Classes []ClassRef `json:"classes"`
}

// TeachingClassInput is the create/update payload for a teaching class. ClassIDs
// is the full set of member admin classes (replace semantics on update).
type TeachingClassInput struct {
	Name     string  `json:"name"`
	Note     string  `json:"note"`
	ClassIDs []int64 `json:"classIds"`
}
