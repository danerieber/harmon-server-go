package main

import (
	"bufio"
	"io"
	"io/fs"
	"os"
)

var dbPerm fs.FileMode = 0700

func dbInit() {
	os.MkdirAll(DataDir, dbPerm)
	os.Mkdir(DataDir+"/message", dbPerm)
	os.Mkdir(DataDir+"/token_to_user_id", dbPerm)
	os.Mkdir(DataDir+"/username_to_user_id", dbPerm)
	os.Mkdir(DataDir+"/user", dbPerm)
	os.Mkdir(DataDir+"/chat_messages", dbPerm)
	os.Mkdir(DataDir+"/image", dbPerm)
	os.Mkdir(DataDir+"/settings", dbPerm)
	os.Mkdir(DataDir+"/developer", dbPerm)
}

func dbTablePath(table string) string {
	return DataDir + "/" + table
}

func dbPath(table, key string) string {
	return dbTablePath(table) + "/" + key
}

func dbRead(table, key string) (value []byte, ok bool) {
	value, err := os.ReadFile(dbPath(table, key))
	return value, err == nil
}

func dbExists(table, key string) bool {
	_, err := os.Stat(dbPath(table, key))
	return err == nil
}

func dbReadAll(table string) (values map[string]([]byte), ok bool) {
	files, err := os.ReadDir(dbTablePath(table))
	if err != nil {
		return nil, false
	}

	values = map[string]([]byte){}

	for _, file := range files {
		values[file.Name()], ok = dbRead(table, file.Name())
		if !ok {
			return nil, false
		}
	}

	return values, true
}

// Constructs a valid JSON array from a portion of a file containing one JSON object per line
func dbReadEntries(table, key string, offset int64, whence int, total int) (value []byte, newOffset int64, newTotal int, ok bool) {
	file, err := os.OpenFile(dbPath(table, key), os.O_RDONLY, dbPerm)
	if err != nil {
		return nil, 0, 0, false
	}
	defer file.Close()

	newOffset, err = file.Seek(int64(offset), whence)

	// If the offset is invalid, just start at the beginning of the file.
	skip := true
	if err != nil {
		newOffset, err = file.Seek(0, io.SeekStart)
		// We don't need to skip the first line since it will be valid JSON
		skip = false
		if err != nil {
			return nil, 0, 0, false
		}
	}

	scanner := bufio.NewScanner(file)
	// Allocate enough space for JSON objects with '[' and ']' characters
	data := make([]byte, total+2)
	data[0] = '[' // Open bracket for new JSON array
	i := 1
	for scanner.Scan() {
		// Skip first line since it is probably invalid JSON (we most likely seeked into the middle of a line)
		if skip {
			skip = false
			continue
		}
		if i >= total {
			i = total
			break
		}
		bytes := scanner.Bytes()
		end := i + len(bytes)
		if end > total {
			break
		}
		if len(bytes) > 0 {
			copy(data[i:end], bytes)
			data[end] = ',' // Insert commas to construct array
		}
		i = end + 1
	}
	data[i-1] = ']' // Replace very last comma to finish closing array
	return data[0:i], newOffset, i - 1, true
}

func dbWrite(table, key string, value []byte) bool {
	return os.WriteFile(dbPath(table, key), value, dbPerm) == nil
}

func dbAppend(table, key string, value []byte) bool {
	file, err := os.OpenFile(dbPath(table, key), os.O_APPEND|os.O_CREATE|os.O_WRONLY, dbPerm)
	if err != nil {
		return false
	}
	defer file.Close()
	if _, err := file.Write(value); err != nil {
		return false
	}
	return true
}

func dbDelete(table, key string) {
	os.Remove(dbPath(table, key))
}
