package ordnung

import (
	"crypto/rand"
	"image"
	_ "image/jpeg"
	"os"
	"sync"
	"testing"
	"time"
)

func createRandomImage() (created *image.NRGBA) {
	rect := image.Rect(0, 0, 100, 100)
	pix := make([]uint8, rect.Dx()*rect.Dy()*4)
	rand.Read(pix)
	created = &image.NRGBA{
		Pix:    pix,
		Stride: rect.Dx() * 4,
		Rect:   rect,
	}
	return
}

func TestImage_ExtractDate(t *testing.T) {
	f, _ := os.CreateTemp("", "sample.jpg")
	type fields struct {
		OriginalName string
		NewName      string
		Date         time.Time
		Process      bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Image{
				OriginalName: tt.fields.OriginalName,
				NewName:      tt.fields.NewName,
				Date:         tt.fields.Date,
				Process:      tt.fields.Process,
			}
			if err := i.ExtractDate(); (err != nil) != tt.wantErr {
				t.Errorf("Image.ExtractDate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestImage_GenerateNewName(t *testing.T) {
	type fields struct {
		OriginalName string
		NewName      string
		Date         time.Time
		Process      bool
	}
	type args struct {
		pattern  string
		newNames *map[string]int
		mutex    *sync.Mutex
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Image{
				OriginalName: tt.fields.OriginalName,
				NewName:      tt.fields.NewName,
				Date:         tt.fields.Date,
				Process:      tt.fields.Process,
			}
			i.GenerateNewName(tt.args.pattern, tt.args.newNames, tt.args.mutex)
		})
	}
}

func TestImage_Rename(t *testing.T) {
	type fields struct {
		OriginalName string
		NewName      string
		Date         time.Time
		Process      bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Image{
				OriginalName: tt.fields.OriginalName,
				NewName:      tt.fields.NewName,
				Date:         tt.fields.Date,
				Process:      tt.fields.Process,
			}
			if err := i.Rename(); (err != nil) != tt.wantErr {
				t.Errorf("Image.Rename() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
