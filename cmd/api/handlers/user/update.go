package handlers

import (
	"context"
	"io"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/users"
	jwt "HuaTug.com/pkg"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func UpdateUser(ctx context.Context, c *app.RequestContext) {
	var v interface{}
	var err error
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	userId := utils.Transfer(v)

	// 构建UpdateUserRequest
	req := &users.UpdateUserRequest{
		UserId: userId,
	}

	// 检查 Content-Type，支持 JSON 和 multipart/form-data
	contentType := string(c.ContentType())
	hlog.Infof("UpdateUser: Content-Type=%s", contentType)

	if contentType == "application/json" || contentType == "" {
		// JSON 格式请求
		var update UpdateParam
		if err := c.BindJSON(&update); err != nil {
			hlog.Errorf("UpdateUser: BindJSON error: %v", err)
			SendResponse(c, errno.ConvertErr(err), nil)
			return
		}
		hlog.Infof("UpdateUser: Parsed JSON - UserName=%s, Sex=%d, Bio=%s", update.UserName, update.Sex, update.Bio)

		if update.UserName != "" {
			req.UserName = update.UserName
		}
		if update.PassWord != "" {
			password, _ := utils.Crypt(update.PassWord)
			req.Password = password
		}
		req.Sex = update.Sex
		if update.Bio != "" {
			req.Bio = update.Bio
		}
	} else {
		// multipart/form-data 格式请求
		var update UpdateParam
		if err := c.Bind(&update); err != nil {
			hlog.Errorf("UpdateUser: Bind error: %v", err)
			SendResponse(c, errno.ConvertErr(err), nil)
			return
		}
		hlog.Infof("UpdateUser: Parsed Form - UserName=%s, Sex=%d, Bio=%s", update.UserName, update.Sex, update.Bio)

		if update.UserName != "" {
			req.UserName = update.UserName
		}
		if update.PassWord != "" {
			password, _ := utils.Crypt(update.PassWord)
			req.Password = password
		}
		req.Sex = update.Sex
		if update.Bio != "" {
			req.Bio = update.Bio
		}

		// 尝试获取上传的文件（可选）
		uploadData, err := c.FormFile("file")
		if err == nil && uploadData != nil {
			file, err := uploadData.Open()
			if err != nil {
				SendResponse(c, errno.ConvertErr(err), nil)
				return
			}
			defer file.Close()

			fileContent, err := io.ReadAll(file)
			if err != nil {
				SendResponse(c, errno.ConvertErr(err), nil)
				return
			}

			req.Data = fileContent
			req.Filesize = uploadData.Size
		}
	}

	resp, err := rpc.UpdateUser(ctx, req)
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	SendResponse(c, errno.Success, resp)
}
