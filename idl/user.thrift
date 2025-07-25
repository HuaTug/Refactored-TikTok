namespace go users

include "base.thrift"


struct CreateUserRequest{
    1: string user_name      (api.body="user_name", api.form="user_name", api.vd="(len($) > 0 && len($) < 100)")
    2: string password (api.body="password", api.form="password", api.vd="len($)>5 &&len($)<12")
    3: string email (api.body="email", api.form="email", api.vd="(len($)>3 && len($) < 12)")
    4: i64 sex (api.body="sex", api.form="sex", api.vd="$ == 0 || $ == 1")
}

struct CreateUserResponse{ 
    1:base.Status base 
}

struct QueryUserRequest{
    1: optional string Keyword (api.body="keyword", api.form="keyword", api.query="keyword")
    2: i64 page (api.body="page", api.form="page", api.query="page", api.vd="$ > 0")
    3: i64 page_size (api.body="page_size", api.form="page_size", api.query="page_size", api.vd="($ > 0 || $ <= 100)")
}

struct QueryUserResponse{
    1: base.Status base
    3: list<base.User> users
    4: i64 totoal
}   

struct DeleteUserRequest{
    1: i64 userId
}

struct DeleteUserResponse{
    1: base.Status base
}

struct UpdateUserRequest{
    1: string user_name (api.body="user_name", api.form="user_name", api.vd="(len($) > 0 && len($) < 100)")
    2: i64 userId
    3: string password (api.body="password", api.form="password", api.vd="(len($)>5 &&len($)<12)")
    4: binary data
    5: i64 filesize
}

struct UpdateUserResponse{
    1: base.Status base
    2: base.User data
}

struct LoginUserResquest{
    1: string user_name   (api.body="user_name", api.form="user_name", api.vd="(len($)>3&&len($)<12)")
    2: string Password   (api.body="password", api.form="password", api.vd="(len($)>5&&len($)<10)")
    3: string Email      (api.body="email", api.form="email", api.vd="(len($)>3&&len($)<12)")
}

struct LoginUserResponse{
    1: base.Status base
    2: string token
    3: string RefreshToken
    4: base.User user
}

struct CheckUserExistsByIdRequst{
    1: i64 userId
}
struct CheckUserExistsByIdResponse{
    1: base.Status base
    2: bool exists
}

struct GetUserInfoRequest{
    1: i64 userId
}
struct GetUserInfoResponse{
    1: base.Status base
    2: base.User User
}

// 对验证码进行验证
struct VerifyCodeRequest{
    1: string code
    2: string email
}
struct VerifyCodeResponse{
    1: base.Status base
}

// 用户在登录时，需要发送验证码（以完成双重认证）
struct SendCodeRequest{
    1: string email
}
struct SendCodeResponse{
    1: base.Status base
}

// 忘记密码功能
struct ForgotPasswordRequest{
    1: string email      (api.body="email", api.form="email", api.vd="(len($)>3&&len($)<50)")
}
struct ForgotPasswordResponse{
    1: base.Status base
    2: string reset_token  // 重置密码的令牌，有效期通常为15-30分钟
}

struct ResetPasswordRequest{
    1: string reset_token (api.body="reset_token", api.form="reset_token", api.vd="(len($)>0)")
    2: string new_password (api.body="new_password", api.form="new_password", api.vd="(len($)>5&&len($)<20)")
}
struct ResetPasswordResponse{
    1: base.Status base
}

// 获取当前用户信息（不需要传用户ID）
struct GetMyProfileRequest{
    // 从JWT token中获取用户ID，所以不需要参数
}
struct GetMyProfileResponse{
    1: base.Status base
    2: base.User user
}

// 上传头像预签名URL
struct GetAvatarUploadUrlRequest{
    1: string file_extension  (api.body="file_extension", api.form="file_extension", api.vd="(len($)>0)")  // 文件扩展名如 ".jpg", ".png"
}
struct GetAvatarUploadUrlResponse{
    1: base.Status base
    2: string upload_url      // 预签名上传URL
    3: string access_url      // 上传成功后的访问URL
    4: i64 expires_in         // 上传URL过期时间（秒）
}

struct UpdateAvatarRequest{
    1: string avatar_url      (api.body="avatar_url", api.form="avatar_url", api.vd="(len($)>0)")  // 上传完成后的文件URL
}
struct UpdateAvatarResponse{
    1: base.Status base
    2: base.User user
}

// 修改密码功能（已登录用户）
struct ChangePasswordRequest{
    1: string old_password    (api.body="old_password", api.form="old_password", api.vd="(len($)>5&&len($)<20)")     // 旧密码
    2: string new_password    (api.body="new_password", api.form="new_password", api.vd="(len($)>5&&len($)<20)")     // 新密码
    3: string confirm_password (api.body="confirm_password", api.form="confirm_password", api.vd="(len($)>5&&len($)<20)") // 确认新密码
}
struct ChangePasswordResponse{
    1: base.Status base
}

service UserService {
   // ========== V1版本API（现有功能） ==========
   UpdateUserResponse UpdateUser(1: UpdateUserRequest req)(api.post="/v1/user/update")
   DeleteUserResponse DeleteUser(1: DeleteUserRequest req)(api.delete="/v1/user/delete")
   QueryUserResponse  QueryUser(1: QueryUserRequest req)(api.post="/v1/user/query")
   CreateUserResponse CreateUser(1: CreateUserRequest req)(api.post="/v1/user/create")
   LoginUserResponse  LoginUser(1: LoginUserResquest req)(api.post="/v1/user/login")
   GetUserInfoResponse GetUserInfo(1: GetUserInfoRequest req)(api.get="/v1/user/get")
   CheckUserExistsByIdResponse CheckUserExistsById(1: CheckUserExistsByIdRequst req)(api.post="/v1/user/check")
   VerifyCodeResponse VerifyCode(1: VerifyCodeRequest req)(api.post="/v1/user/verifycode")
   SendCodeResponse SendCode(1: SendCodeRequest req)(api.post="/v1/user/sendcode")
   
   // ========== V2版本API（新增功能） ==========
   ForgotPasswordResponse ForgotPassword(1: ForgotPasswordRequest req)(api.post="/v1/user/forgot-password")
   ResetPasswordResponse ResetPassword(1: ResetPasswordRequest req)(api.post="/v1/user/reset-password")
   GetMyProfileResponse GetMyProfile(1: GetMyProfileRequest req)(api.get="/v1/user/me")
   GetAvatarUploadUrlResponse GetAvatarUploadUrl(1: GetAvatarUploadUrlRequest req)(api.post="/v1/user/avatar/upload-url")
   UpdateAvatarResponse UpdateAvatar(1: UpdateAvatarRequest req)(api.post="/v1/user/avatar/update")
   ChangePasswordResponse ChangePassword(1: ChangePasswordRequest req)(api.post="/v1/user/change-password")
}