package proto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"../logging"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"crypto/sha256"
)

//(357593060571943BP00,GT03.V18.20171019,7,46001)
//GT004802/iAwAUB3pGjkYw44lYotx/Ny+BskWacSFetg+NFGkUfW/zasg+susRsDKD5QZmw=
/*
20180308,chenqw:增加md5和sha1加密算法实现
*/

const (
	base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

var key = []byte("1Gator@3Xqaz@LZC")
var iv = []byte("1234567890123456")

var key_app = []byte("1Apser@3Xqaz@LZC")
var key_iv = []byte("1234567891234567")

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

func GPSAesDecrypt(encrypted []byte) ([]byte, error) {
	var err error
	decrypted := make([]byte, len(encrypted))
	var block cipher.Block
	block, err = aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	decrypter := cipher.NewCTR(block, iv)
	decrypter.XORKeyStream(decrypted, encrypted)
	return (decrypted), nil
}

func AppserAesEncrypt(data string) (string, error) {
	block, err := aes.NewCipher(key_app)
	if err != nil {
		return "", err
	}
	padData := ZeroPadding([]byte(data), block.BlockSize())
	encrypted := make([]byte, len(padData))

	encrypter := cipher.NewCTR(block, key_iv)
	encrypter.XORKeyStream(encrypted, []byte(padData))
	//return string(encrypted), nil
	return string(base64Encode(encrypted)), nil
}

func AppserDecrypt(encrypted string) ([]byte) {
	//logging.Log("recv from appclient des: " + encrypted)
	var err error
	src, err := base64Decode([]byte(encrypted))
	//if err != nil {
	//	return "", err
	//}
	//newdes := encrypted[6:]
	//base64dec,_ := base64Decode([]byte(newdes))

	decrypted := make([]byte, len(src))
	var block cipher.Block
	block, err = aes.NewCipher(key_app)
	if err != nil {
		return nil
	}

	decrypter := cipher.NewCTR(block, key_iv)
	decrypter.XORKeyStream(decrypted, []byte(src))
	decrypted = ZeroUnPadding(decrypted)
	//logging.Log("decrypted: " + string(decrypted))

	return decrypted
}

func CBCEncrypt(plantText []byte) (string, error) {
	block, err := aes.NewCipher(key_app) //选择加密算法
	if err != nil {
		return "", err
	}
	plantText = PKCS7Padding(plantText, block.BlockSize())

	blockModel := cipher.NewCBCEncrypter(block, key)

	ciphertext := make([]byte, len(plantText))

	blockModel.CryptBlocks(ciphertext, plantText)
	//return ciphertext, nil
	return string(base64Encode(ciphertext)), nil
}

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func CBCDecrypt(ciphertext string) ([]byte, error) {
	keyBytes := []byte(key_app)
	block, err := aes.NewCipher(keyBytes) //选择加密算法
	if err != nil {
		return nil, err
	}

	newdes := ciphertext[6:]
	logging.Log("newdes: " + string(newdes))
	base64dec,_ := base64Decode([]byte(newdes))
	blockModel := cipher.NewCBCDecrypter(block, keyBytes)
	plantText := make([]byte, 10240)
	blockModel.CryptBlocks(plantText, []byte(base64dec))
	plantText = PKCS7UnPadding(plantText)
	logging.Log("decrypted: " + string(plantText))
	return plantText, nil
}

func PKCS7UnPadding(plantText []byte) []byte {
	length := len(plantText)
	unpadding := int(plantText[length-1])
	return plantText[:(length - unpadding)]
}

func ZeroPadding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{0}, padding)//用0去填充
	return append(ciphertext, padtext...)
}

func ZeroUnPadding(origData []byte) []byte{
	return bytes.TrimFunc(origData,
		func(r rune )bool{
			return r == rune(0)
		})
}

func CommonAesEncrypt(data []byte) (string,error) {
	encrypted := make([]byte,len(data))
	aesBlockEncrypter,err := aes.NewCipher(key_app)
	if err != nil {
		return  "",err
	}

	aesEncrypter := cipher.NewCFBEncrypter(aesBlockEncrypter,key_iv)
	aesEncrypter.XORKeyStream(encrypted,data)
	return string(base64Encode(encrypted)),nil
}

func CommonDesDecrypt(data string) (Dst string,err error) {
	defer func() {
		//error handle
		if e := recover();e != nil {
			err = e.(error)
		}
	}()

	src,err := base64Decode([]byte(data))
	if err != nil {
		return "",nil
	}
	decrypted := make([]byte,len(data))
	aesBlockDecrypter,err := aes.NewCipher(key_app)
	if err != nil {
		return "",nil
	}

	aesDecrypter := cipher.NewCFBDecrypter(aesBlockDecrypter, key_iv)
	aesDecrypter.XORKeyStream(decrypted, src)
	return string(decrypted), nil
}

func Md5Encrypt(data string) (string,error) {
	w := md5.New()
	io.WriteString(w,data)
	md5str2 := fmt.Sprintf("%x", w.Sum(nil))

	return md5str2,nil
}

func SHA1Digest(data string) (string,error) {
	sha1 := sha1.New()
	sha1.Write([]byte(data))

	return hex.EncodeToString(sha1.Sum([]byte(""))),nil
}

func SHA256Digest(data string) (string ,error) {
	sha256 := sha256.New()
	sha256.Write([]byte(data))

	return hex.EncodeToString(sha256.Sum([]byte(""))),nil
}