package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
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

	fmt.Println("uploading video", videoID, "by user", userID)

	maxSize := 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxSize))

	vid, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't upload Video", err)
		return
	}

	if vid.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't upload Video", err)
		return
	}

	mediaType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only supported mp4 file type", err)
		return
	}

	fileExt := strings.Split(mediaType, "/")[1]
	key := make([]byte, 32)
	rand.Read(key)
	fileName := fmt.Sprintf("%s.%s", base64.RawURLEncoding.EncodeToString(key), fileExt)
	temp, err := os.CreateTemp("", fileName)
	defer os.Remove(temp.Name())
	defer temp.Close()

	io.Copy(temp, file)
	temp.Seek(0, io.SeekStart)

	processVidPath, err := processVideoForFastStart(temp.Name())
	processIOReader, err := os.Open(processVidPath)
	defer os.Remove(processVidPath)
	defer processIOReader.Close()

	aspectRatio, err := getVideoAspectRatio(temp.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't upload video", err)
		return
	}

	var prefix string
	if aspectRatio == "16:9" {
		prefix = "landscape"
	} else if aspectRatio == "9:16" {
		prefix = "portrait"
	} else {
		prefix = "other"
	}
	objectKey := prefix + "/" + fileName

	cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &objectKey,
		Body:        processIOReader,
		ContentType: &mediaType,
	})

	vidURL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, objectKey)
	vid.VideoURL = &vidURL

	err = cfg.db.UpdateVideo(vid)

	respondWithJSON(w, http.StatusOK, vid)
}
