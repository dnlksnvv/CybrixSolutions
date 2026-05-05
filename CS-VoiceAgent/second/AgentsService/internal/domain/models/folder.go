package models

// Folder — обычная (не виртуальная) папка внутри workspace.
// Важно: список агентов в папке не хранится в Folder; принадлежность агента папке
// определяется только полем `Agent.folder_id` (см. ТЗ).
type Folder struct {
	FolderID    string `json:"folder_id" bson:"folder_id"`
	WorkspaceID string `json:"workspace_id" bson:"workspace_id"`
	Name        string `json:"name" bson:"name"`
	CreatedAt   int64  `json:"created_at" bson:"created_at"`
	UpdatedAt   int64  `json:"updated_at" bson:"updated_at"`
}

