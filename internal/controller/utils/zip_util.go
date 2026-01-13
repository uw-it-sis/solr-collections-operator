package utils

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
)

// Zip creates a zip archive of the files within the given directory ...
func Zip(dirName string, files embed.FS) ([]byte, error) {

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Iterate through the files in the directory ...
	entries, _ := files.ReadDir(dirName)
	for _, file := range entries {
		var fileName = file.Name()
		// Create an entry in the zip file ...
		w, err := zipWriter.Create(fileName)
		if err != nil {
			return nil, err
		}

		data, _ := files.ReadFile(fmt.Sprintf("%s/%s", dirName, fileName))

		// Write the data into the file ...
		_, err = w.Write(data)
		if err != nil {
			return nil, err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
