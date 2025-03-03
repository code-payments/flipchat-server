package generate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

var (
	ErrFetchURL     = errors.New("failed to fetch URL")
	ErrParseHTML    = errors.New("failed to parse HTML")
	ErrProcessImage = errors.New("failed to process image")
)

type Result struct {
	OriginalURL string
	ContentType commonpb.ContentType

	URL         string
	Title       string
	Description string

	ImageURL    string
	ImageHash   string
	ImageWidth  int
	ImageHeight int
}

// GeneratePreview generates preview data for a given URL.
func FetchPreview(ctx context.Context, urlStr string) (*Result, error) {
	// Fetch the URL content.
	resp, err := fetchURL(ctx, urlStr)
	if err != nil {
		return nil, ErrFetchURL
	}
	defer resp.Body.Close()

	// Determine content type from the response header.
	contentTypeHeader := resp.Header.Get("Content-Type")
	previewContentType := detectContentType(contentTypeHeader)

	var title, description, imageURL string

	switch previewContentType {
	case commonpb.ContentType_CONTENT_TYPE_TEXT:
		// Parse HTML for title, description, and image.
		title, description, imageURL, err = parseHTML(resp.Body, urlStr)
		if err != nil {
			return nil, ErrParseHTML
		}
	case commonpb.ContentType_CONTENT_TYPE_IMAGE:
		title = extractFileNameFromURL(urlStr)
		description = ""
		imageURL = urlStr
	case commonpb.ContentType_CONTENT_TYPE_VIDEO:
		title = extractFileNameFromURL(urlStr)
		description = "Video content"
		imageURL = ""
	case commonpb.ContentType_CONTENT_TYPE_AUDIO:
		title = extractFileNameFromURL(urlStr)
		description = "Audio content"
		imageURL = ""
	case commonpb.ContentType_CONTENT_TYPE_PDF:
		title = extractFileNameFromURL(urlStr)
		description = "PDF document"
		imageURL = ""
	default:
		title = extractFileNameFromURL(urlStr)
		description = ""
		imageURL = ""
	}

	// Fetch and process the image if available.
	imageInfo, err := fetchAndProcessImage(ctx, imageURL)
	if err != nil {
		// If image processing failed, set image URL to empty.
		imageInfo = &commonpb.ImageInfo{}
	}

	// Construct the Preview struct.
	return &Result{
		OriginalURL: urlStr,
		ContentType: previewContentType,
		URL:         urlStr,
		Title:       title,
		Description: description,
		ImageURL:    imageURL,
		ImageHash:   imageInfo.BlurHash,
		ImageWidth:  int(imageInfo.Width),
		ImageHeight: int(imageInfo.Height),
	}, nil
}

// fetchURL retrieves the content of the given URL.
func fetchURL(ctx context.Context, urlStr string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	// Set User-Agent to mimic a real browser.
	req.Header.Set("User-Agent", "FlipchatPreviewBot/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Check for non-200 status codes.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, errors.New(string(body))
	}

	return resp, nil
}
