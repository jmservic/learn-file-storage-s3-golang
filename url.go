package main

import(
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"time"
	"context"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"strings"
	"errors"
)

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignedClient := s3.NewPresignClient(s3Client)
	req, err := presignedClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key: &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error){
	if video.VideoURL == nil {
		return video, nil
	}

	s3Info := strings.Split(*video.VideoURL, ",")
	if len(s3Info) != 2 {
		return video, nil 
	}

	tempURL, err := generatePresignedURL(cfg.s3Client, s3Info[0], s3Info[1], time.Duration(5)*time.Minute)
	if err != nil {
		return video, err
	}

	video.VideoURL = &tempURL
	return video, nil
}
