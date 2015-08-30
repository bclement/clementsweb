package handler

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
)

type FileStorer interface {
	/*
	   Store persists the file using the supplied path
	*/
	Store(dirName, fileName string, file multipart.File, headers *multipart.FileHeader) error
}

/*
LocalStore persists files to the local file system
*/
type LocalStore struct {
	webroot string
}

func NewLocalStore(webroot string) LocalStore {
	return LocalStore{webroot}
}

/*
see FIleStorer interface
*/
func (ls LocalStore) Store(dirName, fileName string,
	file multipart.File, headers *multipart.FileHeader) (err error) {

	dirPath := filepath.Join("static", "comics", dirName)
	absPath := filepath.Join(ls.webroot, dirPath)
	err = os.MkdirAll(absPath, 0700)
	if err == nil {
		cfilePath := filepath.Join(absPath, fileName)
		err = overwriteFile(cfilePath, file)
	}
	return
}

type S3Store struct {
	client   *s3.S3
	bucket   string
	coverDir string
}

func NewS3Store(ds boltq.DataStore) (s S3Store, err error) {
	var awsBucket string
	var credsFile string
	var coverDir string
	err = ds.View(func(tx *bolt.Tx) (e error) {
		b := tx.Bucket([]byte("aws.config"))
		if b != nil {
			awsBucket = getStringFromBucket(b, "bucket")
			coverDir = getStringFromBucket(b, "coverDir")
			credsFile = getStringFromBucket(b, "credsFile")
		} else {
			e = fmt.Errorf("Unable to find config bucket: aws.config")
		}
		return
	})
	if err == nil {
		if awsBucket == "" || credsFile == "" || coverDir == "" {
			err = fmt.Errorf("Unable to find all config params in db")
		} else {
			creds := credentials.NewSharedCredentials(credsFile, "default")
			_, err = creds.Get()
			if err == nil {
				config := &aws.Config{
					Region:           aws.String("us-east-1"),
					Endpoint:         aws.String("s3.amazonaws.com"),
					S3ForcePathStyle: aws.Bool(true),
					Credentials:      creds,
					LogLevel:         aws.LogLevel(0),
				}
				client := s3.New(config)
				s = S3Store{client, awsBucket, coverDir}
			}
		}
	}
	return
}

func getStringFromBucket(b *bolt.Bucket, key string) (value string) {
	bytes := b.Get([]byte(key))
	if bytes != nil {
		value = string(bytes)
	}
	return
}

/*
see FIleStorer interface
*/
func (s S3Store) Store(dirName, fileName string,
	f multipart.File, h *multipart.FileHeader) (err error) {

	size, err := getFileSize(f, h)

	if err == nil {
		key := filepath.Join(s.coverDir, dirName, fileName)
		params := &s3.PutObjectInput{
			Bucket:        aws.String(s.bucket), // Required
			Key:           aws.String(key),      // Required
			ACL:           aws.String("public-read"),
			Body:          f,
			ContentLength: aws.Int64(size),
		}

		contentType := getHeaderValue(h, "Content-Type")
		if contentType != "" {
			params.ContentType = aws.String(contentType)
		}

		_, err = s.client.PutObject(params)
	}
	return
}

func getFileSize(f multipart.File, h *multipart.FileHeader) (size int64, err error) {
	contentLenString := getHeaderValue(h, "Content-Length")
	if contentLenString == "" {
		/* second arg as 2 means offset is from the end */
		size, err = f.Seek(0, 2)
		if err == nil {
			_, err = f.Seek(0, 0)
		}
	} else {
		size, err = strconv.ParseInt(contentLenString, 0, 64)
	}
	return
}

func getHeaderValue(h *multipart.FileHeader, key string) (value string) {
	return h.Header.Get(key)
}

/*
processCover reads in the cover image from the request and stores the file using the storer
*/
func processCover(r *http.Request, comic *Comic, storer FileStorer) (coverPath, status string) {
	formFile, headers, err := r.FormFile("cover")
	if err != nil {
		if err == http.ErrMissingFile {
			if comic.CoverPath == "" {
				status = "Missing cover file"
			} else {
				coverPath = comic.CoverPath
			}
		} else {
			status = fmt.Sprintf("Unable to save cover: %v", err.Error())
		}
		return
	}
	dotIndex := strings.LastIndex(headers.Filename, ".")
	ext := headers.Filename[dotIndex:]
	dirName := comic.SeriesKey()
	issuePart := comic.IssueKey()
	coverPart := comic.CoverKey()
	fileName := fmt.Sprintf("%v_%v%v", issuePart, coverPart, ext)
	coverPath = filepath.Join(dirName, fileName)
	err = storer.Store(dirName, fileName, formFile, headers)
	if err != nil {
		status = err.Error()
	}
	return
}

/*
overwriteFile writes the file to the path on the filesystem
*/
func overwriteFile(path string, src multipart.File) error {

	var err error
	var target *os.File
	if src != nil {
		target, err = os.Create(path)
		if err == nil {
			defer target.Close()
			_, err = io.Copy(target, src)
			if err == nil {
				err = target.Sync()
			}
		}
	} else {
		err = fmt.Errorf("Missing data for file: %v", path)
	}

	return err
}
