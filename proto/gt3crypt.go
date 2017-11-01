package proto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

//(357593060571943BP00,GT03.V18.20171019,7,46001)
//GT004802/iAwAUB3pGjkYw44lYotx/Ny+BskWacSFetg+NFGkUfW/zasg+susRsDKD5QZmw=

const (
	base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

var key = []byte("1Gator@3Xqaz@LZC")
var iv = []byte("1234567890123456")

var coder = base64.NewEncoding(base64Table)

func base64Encode(src []byte) []byte {
	return []byte(coder.EncodeToString(src))
}

func base64Decode(src []byte) ([]byte, error) {
	return coder.DecodeString(string(src))
}

func Gt3DecryptTest()  {
	origData := []byte("(008E0105357593060571398BP30,L,2,000000FF,5,1,4,46000024820000DE127,46000024820000DDF1A,46000024820000DEB0D,46000024820000F160B,150728,152900)")  //("(357593060571943BP00,GT03.V18.20171019,7,46001)")
	encryptedBase64Str, err := Gt3AesEncrypt(origData)
	fmt.Println(string(encryptedBase64Str), err)
	//encryptedBase64Str := "/iAwAUB3pGjkYw44lYotx/Ny+BskWacSFetg+NFGkUfW/zasg+susRsDKD5QZmw="

	decryptedStr, err := Gt3AesDecrypt(encryptedBase64Str)
	fmt.Println(string(decryptedStr), err)


	if string(decryptedStr) != string(origData) {
		fmt.Println("Decrypt failed")
	}else{
		fmt.Println("Decrypt Success")
	}
}

func Gt3AesEncrypt(data []byte) (string, error) {
	encrypted := make([]byte, len(data))
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	encrypter := cipher.NewCTR(block, iv)
	encrypter.XORKeyStream(encrypted, []byte(data))
	return string(base64Encode(encrypted)), nil
}

func Gt3AesDecrypt(encrypted string) ([]byte, error) {
	var err error
	src, err := base64Decode([]byte(encrypted))
	if err != nil {
		return nil, err
	}

	decrypted := make([]byte, len(src))
	var block cipher.Block
	block, err = aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	decrypter := cipher.NewCTR(block, iv)
	decrypter.XORKeyStream(decrypted, src)
	return (decrypted), nil
}