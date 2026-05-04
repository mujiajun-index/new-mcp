package common

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

func GetPagination(c *gin.Context) (page, pageSize int) {
	page = DefaultPage
	pageSize = DefaultPageSize

	if v := c.Query("page"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			page = i
		}
	}
	if v := c.Query("page_size"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			pageSize = i
		}
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return
}

func GetOffset(page, pageSize int) int {
	return (page - 1) * pageSize
}
