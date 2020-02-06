package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

func hash(key string) string {
	hasher := md5.New()

	hasher.Write([]byte(key))
	return hex.EncodeToString(hasher.Sum(nil))
}

// encrypt the data with password
func encrypt(data []byte, password string) ([]byte, error) {
	block, err := aes.NewCipher([]byte(hash(password)))
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	cipherText := gcm.Seal(nonce, nonce, data, nil)

	return cipherText, nil
}

// decrypt the data with password
func decrypt(data []byte, password string) ([]byte, error) {
	key := []byte(hash(password))

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	nonce, cipherText := data[:nonceSize], data[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, err
	}

	return plainText, nil
}

// EncryptFile encrypts the content to a file
func EncryptFile(name string, data []byte, password string) error {
	file, err := os.Create(name)
	if err != nil {
		return err
	}
	defer file.Close()

	// encrypt the data with password
	encrypted, err := encrypt(data, password)
	if err != nil {
		return err
	}

	// write the encrypted text to file
	if _, err := file.Write(encrypted); err != nil {
		return err
	}

	return nil
}

// DecryptFile decrypts the file to content
func DecryptFile(name string, password string) ([]byte, error) {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}
	decrypted, err := decrypt(data, password)
	if err != nil {
		return nil, err
	}

	return decrypted, nil
}

func main() {
	name := "login"
	text := "hello world"
	passwd := "alonlong"

	if err := EncryptFile(name, []byte(text), passwd); err != nil {
		panic(err)
	}
	output, err := DecryptFile(name, passwd)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(output))
}
