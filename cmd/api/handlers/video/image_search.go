package handlers

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// ImageSearchByUpload 上传图片进行以图搜图
func ImageSearchByUpload(ctx context.Context, c *app.RequestContext) {
	// 解析multipart form
	form, err := c.MultipartForm()
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 获取上传的图片文件
	files := form.File["image"]
	if len(files) == 0 {
		SendResponse(c, errno.ErrBind.WithMessage("no image file uploaded"), nil)
		return
	}

	file := files[0]
	
	// 检查文件大小 (最大10MB)
	if file.Size > 10*1024*1024 {
		SendResponse(c, errno.ErrBind.WithMessage("image file too large (max 10MB)"), nil)
		return
	}

	// 检查文件类型
	if !isValidImageType(file.Header.Get("Content-Type")) {
		SendResponse(c, errno.ErrBind.WithMessage("invalid image format"), nil)
		return
	}

	// 读取文件内容
	fileContent, err := readMultipartFile(file)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to read uploaded file: %v", err)
		SendResponse(c, errno.ErrInternalServer.WithMessage("failed to read file"), nil)
		return
	}

	// 解析其他参数
	params := parseImageSearchParams(c)
	
	// 构建RPC请求
	req := &videos.ImageSearchRequest{
		UserId:           params.UserID,
		QueryImageData:   fileContent,
		TopK:             params.TopK,
		SimilarityThreshold: params.SimilarityThreshold,
		SearchScope:      params.SearchScope,
		SearchModel:      params.SearchModel,
		Filters:          buildImageSearchFilters(params),
	}

	// 调用RPC服务
	resp, err := rpc.ImageSearch(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "ImageSearch RPC failed: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 返回结果
	SendResponse(c, errno.Success, map[string]interface{}{
		"results":       resp.Results,
		"total_count":   resp.TotalCount,
		"search_time":   resp.SearchTime,
		"query_feature_id": resp.QueryFeatureId,
		"metadata":      resp.Metadata,
	})
}

// ImageSearchByURL 通过图片URL进行以图搜图
func ImageSearchByURL(ctx context.Context, c *app.RequestContext) {
	var req ImageSearchByURLRequest
	if err := c.Bind(&req); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 参数验证
	if req.ImageURL == "" {
		SendResponse(c, errno.ErrBind.WithMessage("image_url is required"), nil)
		return
	}

	// 设置默认值
	if req.TopK <= 0 || req.TopK > 100 {
		req.TopK = 10
	}
	if req.SearchModel == "" {
		req.SearchModel = "clip"
	}
	if req.SearchScope == "" {
		req.SearchScope = "all"
	}

	// 构建RPC请求
	rpcReq := &videos.ImageSearchRequest{
		UserId:              req.UserID,
		QueryImageUrl:       req.ImageURL,
		TopK:                int32(req.TopK),
		SimilarityThreshold: req.SimilarityThreshold,
		SearchScope:         req.SearchScope,
		SearchModel:         req.SearchModel,
		Filters:             buildImageSearchFiltersFromRequest(&req),
	}

	// 调用RPC服务
	resp, err := rpc.ImageSearch(ctx, rpcReq)
	if err != nil {
		hlog.CtxErrorf(ctx, "ImageSearch RPC failed: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 返回结果
	SendResponse(c, errno.Success, map[string]interface{}{
		"results":       resp.Results,
		"total_count":   resp.TotalCount,
		"search_time":   resp.SearchTime,
		"query_feature_id": resp.QueryFeatureId,
		"metadata":      resp.Metadata,
	})
}

// ExtractImageFeatures 提取图像特征
func ExtractImageFeatures(ctx context.Context, c *app.RequestContext) {
	var req ExtractFeaturesRequest
	if err := c.Bind(&req); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 参数验证
	if req.ImageURL == "" {
		SendResponse(c, errno.ErrBind.WithMessage("image_url is required"), nil)
		return
	}

	// 设置默认模型
	if req.ExtractModel == "" {
		req.ExtractModel = "clip"
	}

	// 构建RPC请求
	rpcReq := &videos.ImageFeatureExtractRequest{
		UserId:       req.UserID,
		ImageUrl:     req.ImageURL,
		ExtractModel: req.ExtractModel,
		ImageFormat:  req.ImageFormat,
		RequestId:    utils.GenerateRequestID(),
	}

	// 调用RPC服务
	resp, err := rpc.ImageFeatureExtract(ctx, rpcReq)
	if err != nil {
		hlog.CtxErrorf(ctx, "ImageFeatureExtract RPC failed: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 返回结果
	SendResponse(c, errno.Success, map[string]interface{}{
		"feature_id":       resp.FeatureId,
		"feature_vector":   resp.FeatureVector,
		"vector_dimension": resp.VectorDimension,
		"model_version":    resp.ModelVersion,
		"processing_time":  resp.ProcessingTime,
	})
}

// BatchImageProcess 批量图像处理
func BatchImageProcess(ctx context.Context, c *app.RequestContext) {
	var req BatchProcessRequest
	if err := c.Bind(&req); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 参数验证
	if len(req.ImageURLs) == 0 {
		SendResponse(c, errno.ErrBind.WithMessage("image_urls is required"), nil)
		return
	}

	// 限制批量处理数量
	if len(req.ImageURLs) > 100 {
		SendResponse(c, errno.ErrBind.WithMessage("too many images (max 100)"), nil)
		return
	}

	// 构建RPC请求
	rpcReq := &videos.BatchImageProcessRequest{
		UserId:        req.UserID,
		ImageUrls:     req.ImageURLs,
		ProcessType:   req.ProcessType,
		CallbackUrl:   req.CallbackURL,
		ProcessParams: req.ProcessParams,
	}

	// 调用RPC服务
	resp, err := rpc.BatchImageProcess(ctx, rpcReq)
	if err != nil {
		hlog.CtxErrorf(ctx, "BatchImageProcess RPC failed: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 返回结果
	SendResponse(c, errno.Success, map[string]interface{}{
		"batch_job_id":      resp.BatchJobId,
		"estimated_time":    resp.EstimatedTime,
		"total_images":      resp.TotalImages,
		"job_status_url":    resp.JobStatusUrl,
	})
}

// SimilarityAnalysis 相似度分析
func SimilarityAnalysis(ctx context.Context, c *app.RequestContext) {
	var req SimilarityAnalysisRequest
	if err := c.Bind(&req); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 参数验证
	if req.ImageURLa == "" || req.ImageURLb == "" {
		SendResponse(c, errno.ErrBind.WithMessage("both image URLs are required"), nil)
		return
	}

	// 设置默认值
	if req.AnalysisType == "" {
		req.AnalysisType = "semantic"
	}
	if req.ModelType == "" {
		req.ModelType = "clip"
	}

	// 构建RPC请求
	rpcReq := &videos.SimilarityAnalysisRequest{
		ImageUrlA:    req.ImageURLa,
		ImageUrlB:    req.ImageURLb,
		AnalysisType: req.AnalysisType,
		ModelType:    req.ModelType,
	}

	// 调用RPC服务
	resp, err := rpc.SimilarityAnalysis(ctx, rpcReq)
	if err != nil {
		hlog.CtxErrorf(ctx, "SimilarityAnalysis RPC failed: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 返回结果
	SendResponse(c, errno.Success, map[string]interface{}{
		"similarity_score":   resp.SimilarityScore,
		"detailed_scores":    resp.DetailedScores,
		"analysis_report":    resp.AnalysisReport,
		"difference_regions": resp.DifferenceRegions,
	})
}

// 辅助函数

// isValidImageType 检查是否为有效的图片类型
func isValidImageType(contentType string) bool {
	validTypes := []string{
		"image/jpeg",
		"image/jpg", 
		"image/png",
		"image/webp",
		"image/bmp",
		"image/gif",
	}
	
	for _, validType := range validTypes {
		if contentType == validType {
			return true
		}
	}
	return false
}

// readMultipartFile 读取multipart文件内容
func readMultipartFile(file *multipart.FileHeader) ([]byte, error) {
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	return io.ReadAll(src)
}

// parseImageSearchParams 解析图片搜索参数
func parseImageSearchParams(c *app.RequestContext) *ImageSearchParams {
	params := &ImageSearchParams{
		UserID:              parseUserID(c),
		TopK:                parseInt(c, "top_k", 10),
		SimilarityThreshold: parseFloat(c, "similarity_threshold", 0.0),
		SearchScope:         parseString(c, "search_scope", "all"),
		SearchModel:         parseString(c, "search_model", "clip"),
	}

	// 解析过滤器参数
	if userIDs := parseString(c, "user_ids", ""); userIDs != "" {
		for _, idStr := range strings.Split(userIDs, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil {
				params.UserIDs = append(params.UserIDs, id)
			}
		}
	}

	if tags := parseString(c, "tags", ""); tags != "" {
		params.Tags = strings.Split(tags, ",")
	}

	params.StartDate = parseString(c, "start_date", "")
	params.EndDate = parseString(c, "end_date", "")

	return params
}

// buildImageSearchFilters 构建搜索过滤器
func buildImageSearchFilters(params *ImageSearchParams) *videos.ImageSearchFilters {
	if params == nil {
		return nil
	}

	return &videos.ImageSearchFilters{
		UserIds:   params.UserIDs,
		Tags:      params.Tags,
		StartDate: params.StartDate,
		EndDate:   params.EndDate,
	}
}

// buildImageSearchFiltersFromRequest 从请求构建过滤器
func buildImageSearchFiltersFromRequest(req *ImageSearchByURLRequest) *videos.ImageSearchFilters {
	return &videos.ImageSearchFilters{
		UserIds:   req.UserIDs,
		Tags:      req.Tags,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}
}

// 解析辅助函数
func parseUserID(c *app.RequestContext) int64 {
	userID, _ := strconv.ParseInt(string(c.FormValue("user_id")), 10, 64)
	return userID
}

func parseInt(c *app.RequestContext, key string, defaultValue int) int {
	if val := string(c.FormValue(key)); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

func parseFloat(c *app.RequestContext, key string, defaultValue float64) float64 {
	if val := string(c.FormValue(key)); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func parseString(c *app.RequestContext, key string, defaultValue string) string {
	if val := string(c.FormValue(key)); val != "" {
		return val
	}
	return defaultValue
}

// 请求结构体定义
type ImageSearchParams struct {
	UserID              int64
	TopK                int
	SimilarityThreshold float64
	SearchScope         string
	SearchModel         string
	UserIDs             []int64
	Tags                []string
	StartDate           string
	EndDate             string
}

type ImageSearchByURLRequest struct {
	UserID              int64     `json:"user_id" form:"user_id"`
	ImageURL            string    `json:"image_url" form:"image_url"`
	TopK                int       `json:"top_k" form:"top_k"`
	SimilarityThreshold float64   `json:"similarity_threshold" form:"similarity_threshold"`
	SearchScope         string    `json:"search_scope" form:"search_scope"`
	SearchModel         string    `json:"search_model" form:"search_model"`
	UserIDs             []int64   `json:"user_ids" form:"user_ids"`
	Tags                []string  `json:"tags" form:"tags"`
	StartDate           string    `json:"start_date" form:"start_date"`
	EndDate             string    `json:"end_date" form:"end_date"`
}

type ExtractFeaturesRequest struct {
	UserID       int64  `json:"user_id" form:"user_id"`
	ImageURL     string `json:"image_url" form:"image_url"`
	ExtractModel string `json:"extract_model" form:"extract_model"`
	ImageFormat  string `json:"image_format" form:"image_format"`
}

type BatchProcessRequest struct {
	UserID        int64             `json:"user_id" form:"user_id"`
	ImageURLs     []string          `json:"image_urls" form:"image_urls"`
	ProcessType   string            `json:"process_type" form:"process_type"`
	CallbackURL   string            `json:"callback_url" form:"callback_url"`
	ProcessParams map[string]string `json:"process_params" form:"process_params"`
}

type SimilarityAnalysisRequest struct {
	ImageURLa    string `json:"image_url_a" form:"image_url_a"`
	ImageURLb    string `json:"image_url_b" form:"image_url_b"`
	AnalysisType string `json:"analysis_type" form:"analysis_type"`
	ModelType    string `json:"model_type" form:"model_type"`
} 