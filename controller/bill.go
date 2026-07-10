package controller

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func billSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "SUCCESS",
		"data":      data,
		"timestamp": time.Now().UnixMilli(),
	})
}

func billError(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, gin.H{
		"code":      code,
		"message":   message,
		"data":      nil,
		"timestamp": time.Now().UnixMilli(),
	})
}

func GetBillAliDayList(c *gin.Context) {
	startAt, endAt, err := model.ParseBillDayRange(c.Query("startDay"), c.Query("endDay"))
	if err != nil {
		billError(c, 400, err.Error())
		return
	}
	items, err := model.GetBillAliDayList(startAt, endAt, c.Query("modelName"))
	if err != nil {
		billError(c, 500, err.Error())
		return
	}
	billSuccess(c, items)
}

func GetBillAliDetailList(c *gin.Context) {
	startAt, endAt, err := model.ParseBillDetailRange(c.Query("startTime"), c.Query("endTime"))
	if err != nil {
		billError(c, 400, err.Error())
		return
	}
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	if pageNum < 1 {
		pageNum = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "100"))
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 500 {
		pageSize = 500
	}
	items, total, err := model.GetBillAliDetailList(startAt, endAt, c.Query("modelName"), c.Query("siteUrl"), pageNum, pageSize)
	if err != nil {
		billError(c, 500, err.Error())
		return
	}
	pages := int64(0)
	if total > 0 {
		pages = int64(math.Ceil(float64(total) / float64(pageSize)))
	}
	billSuccess(c, gin.H{
		"total":   total,
		"size":    pageSize,
		"current": pageNum,
		"pages":   pages,
		"records": items,
	})
}
