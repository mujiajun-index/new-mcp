package router

import "github.com/gin-gonic/gin"

func SetRouter(engine *gin.Engine) {
	SetApiRouter(engine)
}
