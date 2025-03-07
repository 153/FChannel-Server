package main

import "fmt"
import "database/sql"
import _ "github.com/lib/pq"
import	"net/smtp"
import "time"
import "os/exec"
import "os"
import "math/rand"
import "crypto"
import "crypto/rsa"
import "crypto/x509"
import "crypto/sha256"
import "encoding/pem"
import "encoding/base64"
import crand "crypto/rand"
import "io/ioutil"
import "strings"
import "net/http"
import "regexp"

type Verify struct {
	Type string
	Identifier string
	Code string
	Created string
	Board string
}

type VerifyCooldown struct {
	Identifier string
	Code string
	Time int
}

type Signature struct {
	KeyId string
	Headers []string
	Signature string
	Algorithm string	
}

func DeleteBoardMod(db *sql.DB, verify Verify) {
	query := `select code from boardaccess where identifier=$1 and board=$1`

	rows, err := db.Query(query, verify.Identifier, verify.Board)	

	CheckError(err, "could not select code from boardaccess")

	defer rows.Close()

	var code string
	rows.Next()
	rows.Scan(&code)

	if code != "" {
		query := `delete from crossverification where code=$1`
		
		_, err := db.Exec(query, code)
		
		CheckError(err, "could not delete code from crossverification")

		query = `delete from boardaccess where identifier=$1 and board=$2`

		_, err = db.Exec(query, verify.Identifier, verify.Board)		
		
		CheckError(err, "could not delete identifier from boardaccess")				
	}
}

func GetBoardMod(db *sql.DB, identifier string) Verify{
	var nVerify Verify

	query := `select code, board, type, identifier from boardaccess where identifier=$1`

	rows, err := db.Query(query, identifier)	

	CheckError(err, "could not select boardaccess query")

	defer rows.Close()

	rows.Next()
	rows.Scan(&nVerify.Code, &nVerify.Board, &nVerify.Type, &nVerify.Identifier)

	return nVerify
}

func CreateBoardMod(db *sql.DB, verify Verify) {
	pass := CreateKey(50)

	query := `select code from verification where identifier=$1 and type=$2`

	rows, err := db.Query(query, verify.Board, verify.Type)	

	CheckError(err, "could not select verifcaiton query")

	defer rows.Close()

	var code string
	
	rows.Next()
	rows.Scan(&code)

	if code != "" {

		query := `select identifier from boardaccess where identifier=$1 and board=$2`

		rows, err := db.Query(query, verify.Identifier, verify.Board)		
		
		CheckError(err, "could not select idenifier from boardaccess")

		defer rows.Close()

		var ident string
		rows.Next()
		rows.Scan(&ident)

		if ident != verify.Identifier {

			query := `insert into crossverification (verificationcode, code) values ($1, $2)`

			_, err := db.Exec(query, code, pass)			
			
			CheckError(err, "could not insert new crossverification")

			query = `insert into boardaccess (identifier, code, board, type) values ($1, $2, $3, $4)`

			_, err = db.Exec(query, verify.Identifier, pass, verify.Board, verify.Type)
			
			CheckError(err, "could not insert new boardaccess")

			fmt.Printf("Board access - Board: %s, Identifier: %s, Code: %s\n", verify.Board, verify.Identifier, pass)
		}
	}
}

func CreateVerification(db *sql.DB, verify Verify) {
	query := `insert into verification (type, identifier, code, created) values ($1, $2, $3, $4)`

	_, err := db.Exec(query, verify.Type, verify.Identifier, verify.Code, time.Now().UTC().Format(time.RFC3339))	

	CheckError(err, "error creating verify")
}

func GetVerificationByEmail(db *sql.DB, email string) Verify {
	var verify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1`

	rows, err := db.Query(query, email)	

	defer rows.Close()

	CheckError(err, "error getting verify by email query")		

	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board)

		CheckError(err, "error getting verify by email scan")				
	}
	
	return verify
}

func GetVerificationByCode(db *sql.DB, code string) Verify {
	var verify Verify

	query := `select type, identifier, code, board from boardaccess where code=$1`

	rows, err := db.Query(query, code)	

	defer rows.Close()

	if err != nil {
		CheckError(err, "error getting verify by code query")
		return verify
	}

	for rows.Next() {
		err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board)

		CheckError(err, "error getting verify by code scan")				
	}
	
	return verify
}

func GetVerificationCode(db *sql.DB, verify Verify) Verify {
	var nVerify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1 and board=$2`

	rows, err := db.Query(query, verify.Identifier, verify.Board)	

	defer rows.Close()

	if err != nil {
		CheckError(err, "error getting verify by code query")
		return verify
	}

	for rows.Next() {
		err := rows.Scan(&nVerify.Type, &nVerify.Identifier, &nVerify.Code, &nVerify.Board)

		CheckError(err, "error getting verify by code scan")				
	}
	
	return nVerify
}

func VerifyCooldownCurrent(db *sql.DB, auth string) VerifyCooldown {
	var current VerifyCooldown

	query := `select identifier, code, time from verificationcooldown where code=$1`

	rows, err := db.Query(query, auth)	

	defer rows.Close()	

	if err != nil {

		query := `select identifier, code, time from verificationcooldown where identifier=$1`

		rows, err := db.Query(query, auth)		

		defer rows.Close()
		
		if err != nil {
			return current
		}
		
		defer rows.Close()

		for rows.Next() {
			err = rows.Scan(&current.Identifier, &current.Code, &current.Time)

			CheckError(err, "error scanning current verify cooldown verification")
		}		
	}

	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&current.Identifier, &current.Code, &current.Time)

		CheckError(err, "error scanning current verify cooldown code")
	}

	return current
}

func VerifyCooldownAdd(db *sql.DB, verify Verify) {
	query := `insert into verficationcooldown (identifier, code) values ($1, $2)`

	_, err := db.Exec(query, verify.Identifier, verify.Code)	

	CheckError(err, "error adding verify to cooldown")
}

func VerficationCooldown(db *sql.DB) {

	query := `select identifier, code, time from verificationcooldown`

	rows, err := db.Query(query)

	defer rows.Close()	

	CheckError(err, "error with verifiy cooldown query ")

	defer rows.Close()

	for rows.Next() {
		var verify VerifyCooldown		
		err = rows.Scan(&verify.Identifier, &verify.Code, &verify.Time)

		CheckError(err, "error with verifiy cooldown scan ")

		nTime := verify.Time - 1;

		query = `update set time=$1 where identifier=$2`

		_, err := db.Exec(query, nTime, verify.Identifier)		

		CheckError(err, "error with update cooldown query")

		VerficationCooldownRemove(db)
	}
}

func VerficationCooldownRemove(db *sql.DB) {
	query := `delete from verificationcooldown where time < 1`

	_, err := db.Exec(query)

	CheckError(err, "error with verifiy cooldown remove query ")
}

func SendVerification(verify Verify) {

	fmt.Println("sending email")

	from := SiteEmail
	pass := SiteEmailPassword
	to := verify.Identifier
	body := fmt.Sprintf("You can use either\r\nEmail: %s \r\n Verfication Code: %s\r\n for the board %s", verify.Identifier, verify.Code, verify.Board)

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: Image Board Verification\n\n" +
		body

	err := smtp.SendMail(SiteEmailServer + ":" + SiteEmailPort,
		smtp.PlainAuth("", from, pass, SiteEmailServer),
		from, []string{to}, []byte(msg))


	CheckError(err, "error with smtp")
}

func IsEmailSetup() bool {
	if SiteEmail == "" {
		return false
	}

	if SiteEmailPassword == "" {
		return false
	}

	if SiteEmailServer == "" {
		return false
	}

	if SiteEmailPort == "" {
		return false
	}	
	
	return true
}

func HasAuth(db *sql.DB, code string, board string) bool {

	verify := GetVerificationByCode(db, code)

	if verify.Board == Domain || (HasBoardAccess(db, verify) && verify.Board == board) {
		return true
	}

	return false;
}

func HasAuthCooldown(db *sql.DB, auth string) bool {
	current := VerifyCooldownCurrent(db, auth)
	if current.Time > 0 {
		return true
	}

	fmt.Println("has auth is false")	
	return false
}

func GetVerify(db *sql.DB, access string) Verify {
	verify := GetVerificationByCode(db, access)

	if verify.Identifier == "" {
		verify = GetVerificationByEmail(db, access)
	}

	return verify
}

func CreateNewCaptcha(db *sql.DB){
	id   := RandomID(8)
	file := "public/" + id + ".png"
	
	for true {
		if _, err := os.Stat("./" + file); err == nil {
			id   = RandomID(8)			
			file = "public/" + id + ".png"
		}else{
			break
		}
	}
	
	captcha := Captcha()

	var pattern string
	rnd := fmt.Sprintf("%d", rand.Intn(3))

	srnd := string(rnd)

	switch srnd {
	case "0" :
		pattern = "pattern:verticalbricks"
		break

	case "1" :
		pattern = "pattern:verticalsaw"
		break
		
	case "2" :
		pattern = "pattern:hs_cross"
		break		

	}
	
	cmd := exec.Command("convert", "-size", "200x98", pattern, "-transparent", "white", file)

	err := cmd.Run()

	CheckError(err, "error with captcha first pass")
	
	cmd  = exec.Command("convert", file, "-fill", "blue", "-pointsize", "62", "-annotate", "+0+70", captcha, "-tile", "pattern:left30", "-gravity", "center", "-transparent", "white", file)

	err = cmd.Run()

	CheckError(err, "error with captcha second pass")

	rnd = fmt.Sprintf("%d", rand.Intn(24) - 12)
	
	cmd  = exec.Command("convert", file, "-rotate", rnd, "-wave", "5x35", "-distort", "Arc", "20", "-wave", "2x35", "-transparent", "white", file)

	err = cmd.Run()

	CheckError(err, "error with captcha third pass")	

	var verification Verify
	verification.Type         = "captcha"	
	verification.Code         = captcha
	verification.Identifier = file

	CreateVerification(db, verification)
}

func CreateBoardAccess(db *sql.DB, verify Verify) {
	if(!HasBoardAccess(db, verify)){
		query  := `insert into boardaccess (identifier, board) values($1, $2)`

		_, err := db.Exec(query, verify.Identifier, verify.Board)		

		CheckError(err, "could not instert verification and board into board access")
	}
}

func HasBoardAccess(db *sql.DB, verify Verify) bool {
	query := `select count(*) from boardaccess where identifier=$1 and board=$2`

	rows, err := db.Query(query, verify.Identifier, verify.Board)	

	defer rows.Close()	

	CheckError(err, "could not select boardaccess based on verify")	

	var count int

	rows.Next()
	rows.Scan(&count)

	if(count > 0) {
		return true
	} else {
		return false
	}
}

func BoardHasAuthType(db *sql.DB, board string, auth string) bool {
	authTypes := GetActorAuth(db, board)

	for _, e := range authTypes {
		if(e == auth){
			return true
		}
	}
	
	return false
}

func Captcha() string {
	rand.Seed(time.Now().UTC().UnixNano())
	domain := "ABEFHKMNPQRSUVWXYZ#$&"
	rng := 4
	newID := ""
	for i := 0; i < rng; i++ {
		newID += string(domain[rand.Intn(len(domain))])
	}
	
	return newID
}	

func CreatePem(db *sql.DB, actor Actor) {
	privatekey, err := rsa.GenerateKey(crand.Reader, 2048)
	CheckError(err, "error creating private pem key")

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privatekey)
	
	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	
	privatePem, err := os.Create("./pem/board/" + actor.Name + "-private.pem")
	CheckError(err, "error creating private pem file for " + actor.Name) 
	
	err = pem.Encode(privatePem, privateKeyBlock)
	CheckError(err, "error encoding private pem")

	publickey := &privatekey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publickey)
	CheckError(err, "error Marshaling public key to X509")	
	
	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	
	publicPem, err := os.Create("./pem/board/" + actor.Name + "-public.pem")
	CheckError(err, "error creating public pem file for " + actor.Name)
	
	err = pem.Encode(publicPem, publicKeyBlock)
	CheckError(err, "error encoding public pem")

	_, err = os.Stat("./pem/board/" + actor.Name + "-public.pem")
	if os.IsNotExist(err) {
		CheckError(err, "public pem file for actor does not exist")
	} else {
		StorePemToDB(db, actor)
	}
}

func StorePemToDB(db *sql.DB, actor Actor) {
	query := "select publicKeyPem from actor where id=$1"
	rows, err := db.Query(query, actor.Id)
	
	CheckError(err, "error selecting publicKeyPem id from actor")	

	var result string
	defer rows.Close()
	rows.Next()
	rows.Scan(&result)

	if(result != "") {
		return
	}

	publicKeyPem := actor.Id + "#main-key"
	query = "update actor set publicKeyPem=$1 where id=$2"
	_, err = db.Exec(query, publicKeyPem, actor.Id)
	CheckError(err, "error updating publicKeyPem id to actor")

	file := "./pem/board/" + actor.Name + "-public.pem"
	query = "insert into publicKeyPem (id, owner, file) values($1, $2, $3)"
	_, err = db.Exec(query, publicKeyPem, actor.Id, file)
	CheckError(err, "error creating publicKeyPem for actor ")
}

func ActivitySign(db *sql.DB, actor Actor, signature string) string {
	query := `select file from publicKeyPem where id=$1 `

	rows, err := db.Query(query, actor.PublicKey.Id)

	CheckError(err, "there was error geting actors public key id")

	var file string
	defer rows.Close()
	rows.Next()
	rows.Scan(&file)

	file = strings.ReplaceAll(file, "public.pem", "private.pem")	
	_, err = os.Stat(file)
	if err == nil {
		publickey, err:= ioutil.ReadFile(file)
		CheckError(err, "error reading file")

		block, _ := pem.Decode(publickey)

		pub, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
		rng :=crand.Reader
		hashed := sha256.New()
		hashed.Write([]byte(signature))		
		cipher, _ := rsa.SignPKCS1v15(rng, pub, crypto.SHA256, hashed.Sum(nil))

		return base64.StdEncoding.EncodeToString(cipher)
	}

	return ""
}

func ActivityVerify(actor Actor, signature string, verify string) error {

	sig, _ := base64.StdEncoding.DecodeString(signature)

	if actor.PublicKey.PublicKeyPem == "" {
		actor = FingerActor(actor.Id)
	}

	block, _ := pem.Decode([]byte(actor.PublicKey.PublicKeyPem))
	pub, _ := x509.ParsePKIXPublicKey(block.Bytes)

	hashed := sha256.New()
	hashed.Write([]byte(verify))

	return rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hashed.Sum(nil), sig)
}

func VerifyHeaderSignature(r *http.Request, actor Actor) bool {
	s := ParseHeaderSignature(r.Header.Get("Signature"))

	var method        string
	var path          string
	var host          string
	var date          string
	var digest        string
	var contentLength string	

	var sig string
	for i, e := range s.Headers {

		var nl string
		if i < len(s.Headers) - 1 {
			nl = "\n"
		}
		
		
		if e == "(request-target)" {
			method = strings.ToLower(r.Method)
			path = r.URL.Path
			sig += "(request-target): " + method + " " + path + "" + nl
			continue
		}

		if e == "host" {
			host = r.Host
			sig += "host: " + host + "" + nl
			continue
		}

		if e == "date" {
			date = r.Header.Get("date")
			sig += "date: " + date + "" + nl
			continue
		}

		if e == "digest" {
			digest = r.Header.Get("digest")
			sig += "digest: " + digest + "" + nl
			continue
		}

		if e == "content-length" {
			contentLength = r.Header.Get("content-length")
			sig += "content-length: " + contentLength + "" + nl 
			continue
		}						
	}

	if s.KeyId != actor.PublicKey.Id {
		return false
	}

	t, _ := time.Parse(time.RFC1123, date)

	if(time.Now().UTC().Sub(t).Seconds() > 75) {
		return false
	}
	
	if ActivityVerify(actor, s.Signature, sig) != nil {
		return false
	}
	
	return true
}

func ParseHeaderSignature(signature string) Signature {
	var nsig Signature

	keyId := regexp.MustCompile(`keyId=`)
	headers := regexp.MustCompile(`headers=`)
	sig := regexp.MustCompile(`signature=`)
	algo := regexp.MustCompile(`algorithm=`)	

	signature = strings.ReplaceAll(signature, "\"", "")
	parts := strings.Split(signature, ",")

	for _, e := range parts {
		if keyId.MatchString(e) {
			nsig.KeyId = keyId.ReplaceAllString(e, "")
			continue
		}
		
		if headers.MatchString(e) {
			header := headers.ReplaceAllString(e, "")
			nsig.Headers = strings.Split(header, " ")
			continue
		}

		if sig.MatchString(e) {
			nsig.Signature = sig.ReplaceAllString(e, "")
			continue
		}

		if algo.MatchString(e) {
			nsig.Algorithm = algo.ReplaceAllString(e, "")
			continue
		}		
	}

	return nsig
}
