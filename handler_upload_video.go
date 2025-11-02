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

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error seeking on temporary file", err)
	}

	fileName := getAssetPath(mediaType)
	putInput := s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fileName,
		Body: tempFile,
		ContentType: &contentType,
	} 
	_, err = cfg.s3Client.PutObject(r.Context(), &putInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error uploading file to amazon s3", err)
	}

	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, fileName)  
	video.VideoURL= &videoURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
