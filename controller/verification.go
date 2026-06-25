package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

// SendEmailVerification generates a 6-digit verification code, stores it
// in-memory, and emails it to the requester. Used by the registration flow.
func SendEmailVerification(c *gin.Context) {
	email := strings.TrimSpace(c.Query("email"))
	if email == "" {
		common.Error(c, http.StatusBadRequest, "邮箱地址不能为空")
		return
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		common.Error(c, http.StatusBadRequest, "无效的邮箱地址")
		return
	}

	if !model.IsEmailDomainAllowed(email) {
		common.Error(c, http.StatusBadRequest, "该邮箱域名不在允许列表中")
		return
	}

	if model.IsEmailAlreadyTaken(email) {
		common.Error(c, http.StatusBadRequest, "邮箱地址已被占用")
		return
	}

	code := common.GenerateVerificationCode(6)
	common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)

	systemName := model.GetOptionString("SystemName")
	subject := fmt.Sprintf("%s邮箱验证邮件", systemName)
	content := fmt.Sprintf("<p>您好，你正在进行%s邮箱验证。</p>"+
		"<p>您的验证码为: <strong>%s</strong></p>"+
		"<p>验证码 %d 分钟内有效，如果不是本人操作，请忽略。</p>",
		systemName, code, common.VerificationValidMinutes)

	if err := service.SendEmail(subject, email, content); err != nil {
		common.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.Success(c, nil)
}
