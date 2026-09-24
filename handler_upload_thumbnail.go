package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't Upload Thumbnail", err)
		return
	}
	contentType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't Upload Thumbnail", err)
		return
	}

	if contentType != "image/jpeg" && contentType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "File Type Not Supported, Only Upload jpeg/jpg or png", err)
		return
	}

	fileExt := strings.Split(contentType, "/")[1]
	key := make([]byte, 32)
	rand.Read(key)
	fileName := base64.RawURLEncoding.EncodeToString(key)

	filePath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", fileName, fileExt))

	destination, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error Uploading Thumbnail", err)
		return
	}
	defer destination.Close()

	_, err = io.Copy(destination, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error Uploading Thumbnail", err)
		return
	}

	vid, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couln't Upload Thumbnail", err)
		return
	}

	if vid.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, filePath)

	vid.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(vid)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couln't Upload Thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, vid)
}
