// Package turn contains data transfer types shared by the runtime and its
// adapters. It deliberately has no dependency on provider or gateway.
package turn

// Attachment describes a media attachment carried by a user turn.
// The JSON names are part of the persisted session format and must remain
// stable so existing sessions can be loaded without a migration.
type Attachment struct {
	Type     AttachmentType    `json:"type,omitempty"`
	FileID   string            `json:"file_id,omitempty"`
	FileURL  string            `json:"file_url,omitempty"`
	FilePath string            `json:"file_path,omitempty"`
	FileName string            `json:"file_name,omitempty"`
	MimeType string            `json:"mime_type,omitempty"`
	FileSize int64             `json:"file_size,omitempty"`
	Data     []byte            `json:"data,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AttachmentType identifies the kind of an attachment.
type AttachmentType string

const (
	AttachmentImage    AttachmentType = "image"
	AttachmentAudio    AttachmentType = "audio"
	AttachmentVideo    AttachmentType = "video"
	AttachmentDocument AttachmentType = "document"
)
