package main

import (
	"net/http"
	"github.com/google/uuid"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"fmt"
	"mime"
	"os"
	"io"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"os/exec"
	"bytes"
	"encoding/json"
//	"strconv"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return

	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not your video!!", err)
		return
	}

	fmt.Println("uploading video file for video", videoID, "by user", userID)

	videoReader, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
		return
	}
	defer videoReader.Close()

	contentType := fileHeader.Header.Get("Content-type")
	if contentType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for video", nil)
		return
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if mediaType != "video/mp4" || err != nil {
		respondWithError(w, http.StatusBadRequest, "received an invalid content-type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error creating temporary file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	videoLimiter := http.MaxBytesReader(w, videoReader, 1 << 30) //1 GB
	_, err = io.Copy(tempFile, videoLimiter)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error copying video file to temporary file", err)
		return
	}
	
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting video aspect ratio", err)
		return
	}

	//process video file for fast start
	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error processing video file for fast start", err)
		return
	}

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error opening fast start file", err)
		return
	}
	defer os.Remove(processedFilePath)
	defer processedFile.Close()

	fileName := getAssetPath(mediaType)
	var fileKey string
	switch aspectRatio {
	case "9:16":
		fileKey = "portrait/" + fileName
	case "16:9":
		fileKey = "landscape/" + fileName
	default:
		fileKey = "other/" + fileName
	}

	putInput := s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fileKey,
		Body: processedFile,
		ContentType: &contentType,
	} 
	_, err = cfg.s3Client.PutObject(r.Context(), &putInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error uploading file to amazon s3", err)
	}

//	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, fileKey)  
	videoBucketAndKey := cfg.s3Bucket + "," + fileKey
	video.VideoURL= &videoBucketAndKey

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating video metadata", err)
		return
	}

	video, err = cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating a signed URL for the video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format",
				"json", "-show_streams", filePath)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	videoInfo := struct{
		Streams []struct{
			Width float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"streams"`
	}{}

	err = json.Unmarshal(buffer.Bytes(), &videoInfo)
	if err != nil {
		return "", fmt.Errorf("ffprobe error: %w", err)
	}
	if len(videoInfo.Streams) < 1 {
		return "", fmt.Errorf("video has no associated streams")
	}	
	if videoInfo.Streams[0].Height <= 0 {
		return "", fmt.Errorf("invalid height for stream")
	}
	screenRatio := videoInfo.Streams[0].Width / videoInfo.Streams[0].Height
	// Bad way lol, but I did get it to work.
/*	videoInfo := map[string]any{}
	err = json.Unmarshal(buffer.Bytes(), &videoInfo)
	if err != nil {
		return "", err
	}
	stream := videoInfo["streams"].([]any)[0].(map[string]any)
	width := stream["width"].(float64)
	height := stream["height"].(float64)
	
	screenRatio := width / height */
	if screenRatio >= 0.45 && screenRatio <= 0.67 {
		return "9:16", nil
	}
	if screenRatio >= 1.6 && screenRatio <= 2.0 {
		return "16:9", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy",
				"-movflags", "faststart", "-f", "mp4", outputFilePath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return outputFilePath, nil
}
