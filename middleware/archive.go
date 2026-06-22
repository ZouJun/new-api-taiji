package middleware

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service/archive"
	"github.com/gin-gonic/gin"
)

func Archive(routeMode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager := archive.Current()
		if manager == nil {
			c.Next()
			return
		}
		if !archive.ShouldSample(c, manager) {
			c.Next()
			return
		}

		state := archive.NewState(c, routeMode, manager)
		archive.Attach(c, state)
		headerSnapshot, headerTruncated := common.BuildArchiveRequestHeaderSnapshot(c.Request.Header, manager.HeaderValueMaxLength())
		state.SetRequestHeader(headerSnapshot, headerTruncated)

		load := archive.EvaluateLoad(manager)
		if load.ShouldSkip {
			state.SetSkip(load.Reason, load.Detail, load.CPUThresholdExceeded, load.MemoryThresholdExceeded, load.DiskThresholdExceeded)
			c.Next()
			manifest := state.Snapshot(c.Writer.Status(), common.GetContextKeyBool(c, constant.ContextKeyIsStream))
			manager.Enqueue(c, manifest)
			return
		}

		requestInfo := archive.ObjectInfo{Status: archive.StatusPending, Stage: "downstream_raw"}
		_, transformedBody := c.Get(common.KeyRequestBody)
		_, hasRawBodyStorage := c.Get(common.KeyRawRequestBodyStorage)
		hasRequestBody := transformedBody || hasRawBodyStorage || c.Request.ContentLength > 0 || len(c.Request.TransferEncoding) > 0
		requestContentType := c.GetHeader("Content-Type")
		if requestContentType == "" {
			requestContentType = c.ContentType()
		}
		if !hasRequestBody && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
			requestInfo = archive.ObjectInfo{Status: archive.StatusSkipped, Reason: archive.ReasonNoRequestBody, Stage: "downstream_absent"}
		} else if storage, isRaw, err := common.GetArchiveRequestBodyStorage(c); err != nil {
			requestInfo = archive.ObjectInfo{Status: archive.StatusFailed, Reason: archive.ReasonRequestSpoolFailed, Stage: "downstream_raw"}
			logger.LogWarn(c, "archive request capture failed: "+err.Error())
		} else {
			if !isRaw && transformedBody {
				requestInfo.Stage = "downstream_fallback"
				requestInfo.Fallback = true
			} else if encoding := c.GetString(common.KeyRawRequestContentEncoding); encoding != "" {
				requestInfo.ContentEncoding = encoding
			}
			requestInfo, err = manager.CaptureRequest(storage)
			if !isRaw && transformedBody {
				requestInfo.Stage = "downstream_fallback"
				requestInfo.Fallback = true
			} else if encoding := c.GetString(common.KeyRawRequestContentEncoding); encoding != "" {
				requestInfo.ContentEncoding = encoding
			}
			if err != nil {
				logger.LogWarn(c, "archive request spool failed: "+err.Error())
			}
			if !isRaw && transformedBody {
				if _, err := storage.Seek(0, io.SeekStart); err == nil {
					c.Request.Body = io.NopCloser(storage)
				}
			}
		}
		state.SetRequest(requestInfo, requestContentType)

		tee := archive.NewTeeWriter(c.Writer, manager.SpoolDir(), manager.MaxResponseBytes(), manager.SmallPayloadMaxBytes())
		c.Writer = tee
		c.Next()

		responseInfo := tee.Finish()
		state.SetClientResponse(responseInfo, c.Writer.Header().Get("Content-Type"))
		state.PopulateFromContext(c)
		manifest := state.Snapshot(c.Writer.Status(), common.GetContextKeyBool(c, constant.ContextKeyIsStream))
		manager.Enqueue(c, manifest)
	}
}
