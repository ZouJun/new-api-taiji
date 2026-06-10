package middleware

import (
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service/archive"
	"github.com/gin-gonic/gin"
)

func Archive(routeMode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !archive.Enabled() {
			c.Next()
			return
		}
		manager := archiveManager()
		if manager == nil {
			c.Next()
			return
		}

		state := archive.NewState(c, routeMode)
		archive.Attach(c, state)

		requestInfo := archive.ObjectInfo{Status: archive.StatusPending}
		if storage, err := common.GetBodyStorage(c); err != nil {
			requestInfo = archive.ObjectInfo{Status: archive.StatusFailed, Reason: archive.ReasonRequestSpoolFailed}
			logger.LogWarn(c, "archive request capture failed: "+err.Error())
		} else {
			if _, err := storage.Seek(0, io.SeekStart); err != nil {
				requestInfo = archive.ObjectInfo{Status: archive.StatusFailed, Reason: archive.ReasonRequestSpoolFailed}
				logger.LogWarn(c, "archive request seek failed: "+err.Error())
			} else {
				requestInfo, err = manager.CaptureRequest(storage)
				if err != nil {
					logger.LogWarn(c, "archive request spool failed: "+err.Error())
				}
			}
			if _, err := storage.Seek(0, io.SeekStart); err == nil {
				c.Request.Body = io.NopCloser(storage)
			}
		}
		state.SetRequest(requestInfo, c.ContentType())
		manager.WritePlaceholder(state)

		tee := archive.NewTeeWriter(c.Writer, state, manager.SpoolDir(), manager.MaxResponseBytes())
		c.Writer = tee
		c.Next()

		responseInfo := tee.Finish()
		state.SetResponse(responseInfo, c.Writer.Header().Get("Content-Type"))
		manifest := state.Snapshot(c.Writer.Status(), common.GetContextKeyBool(c, constant.ContextKeyIsStream))
		manager.Enqueue(c, manifest)
	}
}

func archiveManager() *archive.Manager {
	if !archive.Enabled() {
		return nil
	}
	return archive.Current()
}
