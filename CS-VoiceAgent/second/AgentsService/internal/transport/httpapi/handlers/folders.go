package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type createFolderReq struct {
	Name string `json:"name"`
}

type renameFolderReq struct {
	Name string `json:"name"`
}

func (a *API) ListFolders(c *gin.Context) {
	ws := workspaceID(c)
	items, err := a.deps.Folders.List(c.Request.Context(), ws)
	if err != nil {
		writeError(c, err)
		return
	}
	// Виртуальную папку Template Agents по ТЗ \"можно\" добавлять, но не обязательно — не добавляем, чтобы не придумывать folder_id.
	writeOK(c, http.StatusOK, items)
}

func (a *API) CreateFolder(c *gin.Context) {
	ws := workspaceID(c)
	var req createFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, badJSON(err))
		return
	}
	f, err := a.deps.Folders.Create(c.Request.Context(), ws, req.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusCreated, f)
}

func (a *API) RenameFolder(c *gin.Context) {
	ws := workspaceID(c)
	folderID := c.Param("folderId")
	var req renameFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, badJSON(err))
		return
	}
	f, err := a.deps.Folders.Rename(c.Request.Context(), ws, folderID, req.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, f)
}

func (a *API) DeleteFolder(c *gin.Context) {
	ws := workspaceID(c)
	folderID := c.Param("folderId")
	if err := a.deps.Folders.Delete(c.Request.Context(), ws, folderID); err != nil {
		writeError(c, err)
		return
	}
	writeNoContent(c)
}

