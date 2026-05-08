package confluence

// Page is a Confluence Cloud v2 page resource.
// See: https://developer.atlassian.com/cloud/confluence/rest/v2/api-group-page/
type Page struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"`
	Title    string    `json:"title"`
	SpaceID  string    `json:"spaceId"`
	ParentID string    `json:"parentId"`
	Version  Version   `json:"version"`
	Body     *BodyRead `json:"body,omitempty"`
}

// BodyRead is the nested body shape returned by v2 page reads when a
// body-format is requested. Only the requested format is populated.
type BodyRead struct {
	Storage *BodyStorageRead `json:"storage,omitempty"`
}

// BodyStorageRead is the storage-format payload inside BodyRead.
type BodyStorageRead struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

// Version tracks page version for optimistic locking.
type Version struct {
	Number  int    `json:"number"`
	Message string `json:"message,omitempty"`
}

// BodyWrite is the body shape accepted by v2 create/update requests (flat form).
// v2 responses use a different, wrapped shape, so we only use this for writes.
type BodyWrite struct {
	Representation string `json:"representation"` // "storage"
	Value          string `json:"value"`
}

// CreateRequest is the body for POST /api/v2/pages.
type CreateRequest struct {
	SpaceID  string    `json:"spaceId"`
	Status   string    `json:"status"` // "current"
	Title    string    `json:"title"`
	ParentID string    `json:"parentId,omitempty"`
	Body     BodyWrite `json:"body"`
}

// UpdateRequest is the body for PUT /api/v2/pages/{id}.
type UpdateRequest struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"` // "current"
	Title    string    `json:"title"`
	SpaceID  string    `json:"spaceId,omitempty"`
	ParentID string    `json:"parentId,omitempty"`
	Body     BodyWrite `json:"body"`
	Version  Version   `json:"version"`
}

// PageList is the paginated collection response for v2 page search.
type PageList struct {
	Results []Page `json:"results"`
}

// Space represents a v2 Confluence space.
type Space struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// SpaceList is the paginated collection response for v2 space lookup.
type SpaceList struct {
	Results []Space `json:"results"`
}

// Attachment is the v1 Confluence attachment resource (v2 attachment endpoints
// don't cover upload reliably, so we use the v1 `/rest/api/content` endpoints
// for attachment management).
type Attachment struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// AttachmentList is the paginated v1 response for GET child/attachment.
type AttachmentList struct {
	Results []Attachment `json:"results"`
}
