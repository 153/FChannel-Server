package main

import "fmt"
import "database/sql"
import _ "github.com/lib/pq"
import "time"
import "os"
import "strings"
import "sort"

func GetActorFromDB(db *sql.DB, id string) Actor {
	var nActor Actor

	query :=`select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where id=$1`

	rows, err := db.Query(query, id)

	if CheckError(err, "could not get actor from db query") != nil {
		return nActor
	}

	var publicKeyPem string
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)
		CheckError(err, "error with actor from db scan ")
	}

	nActor.PublicKey = GetActorPemFromDB(db, publicKeyPem)

	return nActor	
}

func GetActorByNameFromDB(db *sql.DB, name string) Actor {
	var nActor Actor

	query :=`select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where name=$1`

	rows, err := db.Query(query, name)

	if CheckError(err, "could not get actor from db query") != nil {
		return nActor
	}

	var publicKeyPem string	
	defer rows.Close()	
	for rows.Next() {
		err = rows.Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)
		CheckError(err, "error with actor from db scan ")
	}

	nActor.PublicKey = GetActorPemFromDB(db, publicKeyPem)	

	return nActor	
}

func CreateNewBoardDB(db *sql.DB, actor Actor) Actor{

	query := `insert into actor (type, id, name, preferedusername, inbox, outbox, following, followers, summary, restricted) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := db.Exec(query, actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary, actor.Restricted)

	if err != nil {
		fmt.Println("board exists")
	} else {
		fmt.Println("board added")
		for _, e := range actor.AuthRequirement {
			query  = `insert into actorauth (type, board) values ($1, $2)`
			_, err := db.Exec(query, e, actor.Name)
			CheckError(err, "auth exists")
		}

		var verify Verify

		verify.Identifier = actor.Id
		verify.Code  = CreateKey(50)
		verify.Type  = "admin"

		CreateVerification(db, verify)

		verify.Identifier = actor.Id
		verify.Code  = CreateKey(50)
		verify.Type  = "janitor"

		CreateVerification(db, verify)

		verify.Identifier = actor.Id
		verify.Code  = CreateKey(50)
		verify.Type  = "post"

		CreateVerification(db, verify)		

		var nverify Verify
		nverify.Board = actor.Id
		nverify.Identifier = "admin"
		nverify.Type = "admin"
		CreateBoardMod(db, nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "janitor"
		nverify.Type = "janitor"
		CreateBoardMod(db, nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "post"
		nverify.Type = "post"
		CreateBoardMod(db, nverify)

		CreatePem(db, actor)
		
		if actor.Name != "main" {
			var nObject ObjectBase
			var nActivity Activity

			nActor := GetActorFromDB(db, Domain)
			nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
			nActivity.Type = "Follow"
			nActivity.Actor = &nActor
			nActivity.Object = &nObject

			mActor := GetActorFromDB(db, actor.Id)
			nActivity.Object.Actor = mActor.Id
			nActivity.To = append(nActivity.To, actor.Id)

			response := AcceptFollow(nActivity)
			SetActorFollowingDB(db, response)
			MakeActivityRequest(db, nActivity)
		}

	}

	return actor
}

func GetBoards(db *sql.DB) []Actor {

	var board []Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers FROM actor`
	
	rows, err := db.Query(query)

	CheckError(err, "could not get boards from db query")

	defer rows.Close()	
	for rows.Next(){
		var actor = new(Actor)
		
		err = rows.Scan(&actor.Type, &actor.Id, &actor.Name, &actor.PreferredUsername, &actor.Inbox, &actor.Outbox, &actor.Following, &actor.Followers)
		
		if err !=nil{
			panic(err)
		}

		board = append(board, *actor)
	}

	return board
}

func WriteObjectToDB(db *sql.DB, obj ObjectBase) ObjectBase {
	obj.Id = fmt.Sprintf("%s/%s", obj.Actor, CreateUniqueID(db, obj.Actor))
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			obj.Preview.Id = fmt.Sprintf("%s/%s", obj.Actor, CreateUniqueID(db, obj.Actor))
			obj.Preview.Published = time.Now().UTC().Format(time.RFC3339)
			obj.Preview.Updated = time.Now().UTC().Format(time.RFC3339)
			obj.Preview.AttributedTo = obj.Id
			WritePreviewToDB(db, *obj.Preview)
		}
		
		for i, _ := range obj.Attachment {
			obj.Attachment[i].Id = fmt.Sprintf("%s/%s", obj.Actor, CreateUniqueID(db, obj.Actor))			
			obj.Attachment[i].Published = time.Now().UTC().Format(time.RFC3339)
			obj.Attachment[i].Updated = time.Now().UTC().Format(time.RFC3339)
			obj.Attachment[i].AttributedTo = obj.Id
			WriteAttachmentToDB(db, obj.Attachment[i])
			WriteActivitytoDBWithAttachment(db, obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoDB(db, obj)
	}

	WriteObjectReplyToDB(db, obj)
	WriteWalletToDB(db, obj)

	return obj
}

func WriteObjectUpdatesToDB(db *sql.DB, obj ObjectBase) {
	query := `update activitystream set updated=$1 where id=$2`
	
	_, e := db.Exec(query, time.Now().UTC().Format(time.RFC3339), obj.Id)
	
	if e != nil{
		fmt.Println("error inserting updating inreplyto")
		panic(e)			
	}

	query = `update cacheactivitystream set updated=$1 where id=$2`
	
	_, e = db.Exec(query, time.Now().UTC().Format(time.RFC3339), obj.Id)
	
	if e != nil{
		fmt.Println("error inserting updating cache inreplyto")
		panic(e)			
	}			
}

func WriteObjectReplyToLocalDB(db *sql.DB, id string, replyto string) {
	query := `insert into replies (id, inreplyto) values ($1, $2)`

	_, err := db.Exec(query, id, replyto)

	CheckError(err, "Could not insert local reply query")

	query = `select inreplyto from replies where id=$1`

	rows, err := db.Query(query,replyto)

	CheckError(err, "Could not query select inreplyto")

	defer rows.Close()

	for rows.Next() {
		var val string
		rows.Scan(&val)
		if val == "" {
			updated := time.Now().UTC().Format(time.RFC3339)
			query := `update activitystream set updated=$1 where id=$2`

			_, err := db.Exec(query, updated, replyto)

			CheckError(err, "error with updating replyto updated at date")
		}
	}
}

func WriteObjectReplyToDB(db *sql.DB, obj ObjectBase) {
	for _, e := range obj.InReplyTo {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, e.Id)

		CheckError(err, "error selecting replies db")

		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {		
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			_, err := db.Exec(query, obj.Id, e.Id)			


			CheckError(err, "error inserting replies db")			
		}

		update := true
		for _, e := range obj.Option {
			if e == "sage" || e == "nokosage" {
				update = false
				break
			}
		}
		
		if update {
			WriteObjectUpdatesToDB(db, e)
		}			
	}

	if len(obj.InReplyTo) < 1 {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, "")

		CheckError(err, "error selecting replies db")

		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			_, err := db.Exec(query, obj.Id, "")			

			CheckError(err, "error inserting replies db")
		}
	}
}

func WriteActorObjectReplyToDB(db *sql.DB, obj ObjectBase) {
	for _, e := range obj.InReplyTo {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, e.Id)

		CheckError(err, "error selecting replies db")

		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {		
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			_, err := db.Exec(query, obj.Id, e.Id)			


			CheckError(err, "error inserting replies db")			
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, "")

		CheckError(err, "error selecting replies db")

		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			_, err := db.Exec(query, obj.Id, "")			

			CheckError(err, "error inserting replies db")
		}
	}
}

func WriteWalletToDB(db *sql.DB, obj ObjectBase) {
	for _, e := range obj.Option { 	
		if e == "wallet" {
			for _, e := range obj.Wallet {
				query := `insert into wallet (id, type, address) values ($1, $2, $3)`

				_, err := db.Exec(query, obj.Id ,e.Type, e.Address)			

				CheckError(err, "error with write wallet query")
			}
			return 
		}
	}	
}

func WriteActivitytoDB(db *sql.DB, obj ObjectBase) {

	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)	
	
	if e != nil{
		fmt.Println("error inserting new activity")
		panic(e)			
	}	
}

func WriteActivitytoDBWithAttachment(db *sql.DB, obj ObjectBase, attachment ObjectBase, preview NestedObjectBase) {
	
	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)	
	
	if e != nil{
		fmt.Println("error inserting new activity with attachment")
		panic(e)			
	}	
}

func WriteAttachmentToDB(db *sql.DB, obj ObjectBase) {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)	
	
	if e != nil{
		fmt.Println("error inserting new attachment")
		panic(e)			
	}
}

func WritePreviewToDB(db *sql.DB, obj NestedObjectBase) {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	
	if e != nil{
		fmt.Println("error inserting new attachment")
		panic(e)			
	}
}

func GetActivityFromDB(db *sql.DB, id string) Collection {
	var nColl Collection
	var nActor Actor
	var result []ObjectBase

	nColl.Actor = &nActor

	query := `select  actor, id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from  activitystream where id=$1 order by updated asc`

	rows, err := db.Query(query, id)	

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string	
		var previewID	string
		
		err = rows.Scan(&nColl.Actor.Id, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = actor.Id		

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)		

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetObjectFromDBPage(db *sql.DB, id string, page int) Collection {
	var nColl Collection
	var result []ObjectBase

	query := `select count (x.id) over(), x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 8 offset $2`

	rows, err := db.Query(query, id, page * 8)	

	CheckError(err, "error query object from db")

	var count int
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		
		
		err = rows.Scan(&count, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesDBLimit(db, post, 5)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt		

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nColl.TotalItems = count
	nColl.OrderedItems = result

	return nColl	
}

func GetObjectFromDB(db *sql.DB, id string) Collection {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`

	rows, err := db.Query(query, id)	

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt		

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetObjectFromDBCatalog(db *sql.DB, id string) Collection {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc`	

	rows, err := db.Query(query, id)	

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = &replies

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesCount(db, post)		

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetObjectByIDFromDB(db *sql.DB, postID string) Collection {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 and type='Note') as x`

	rows, err := db.Query(query, postID)	

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)
		
		CheckError(err, "error scan object into post struct")

		actor = GetActorFromDB(db, actor.Id)

		post.Actor = actor.Id

		nColl.Actor = &actor		

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt		

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetInReplyToDB(db *sql.DB, parent ObjectBase) []ObjectBase {
	var result []ObjectBase

	query := `select inreplyto from replies where id =$1` 

	rows, err := db.Query(query, parent.Id)

	CheckError(err, "error with inreplyto db query")

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase

		rows.Scan(&post.Id)

		result = append(result, post)
	}

	return result
}

func GetObjectRepliesDBLimit(db *sql.DB, parent ObjectBase, limit int) (*CollectionBase, int, int) {

	var nColl CollectionBase
	var result []ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published desc limit $2`

	rows, err := db.Query(query, parent.Id, limit)	

	CheckError(err, "error with replies db query")	

	var postCount int
	var attachCount int
	
	defer rows.Close()	
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)
		
		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)

		CheckError(err, "error with replies db scan")

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesRepliesDB(db, post)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)				

		result = append(result, post)			
	}

	nColl.OrderedItems = result

	sort.Sort(ObjectBaseSortAsc(nColl.OrderedItems))			

	return &nColl, postCount, attachCount
}

func GetObjectRepliesDB(db *sql.DB, parent ObjectBase) (*CollectionBase, int, int) {

	var nColl CollectionBase
	var result []ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published asc`	

	rows, err := db.Query(query, parent.Id)	

	CheckError(err, "error with replies db query")

	var postCount int
	var attachCount int

	defer rows.Close()	
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)
		
		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)

		CheckError(err, "error with replies db scan")

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesRepliesDB(db, post)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt		
		
		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)				

		result = append(result, post)			
	}

	nColl.OrderedItems = result

	return &nColl, postCount, attachCount
}

func GetObjectRepliesReplies(db *sql.DB, parent ObjectBase) (*CollectionBase, int, int) {

	var nColl CollectionBase
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' order by updated asc`

	rows, err := db.Query(query, parent.Id)	

	CheckError(err, "error with replies replies db query")

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)

		CheckError(err, "error with replies replies db scan")

		post.Actor = actor.Id

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)				

		result = append(result, post)			
	}

	nColl.OrderedItems = result

	return &nColl, 0, 0	
}

func GetObjectRepliesRepliesDB(db *sql.DB, parent ObjectBase) (*CollectionBase, int, int) {

	var nColl CollectionBase
	var result []ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published asc`	

	rows, err := db.Query(query, parent.Id)	

	CheckError(err, "error with replies replies db query")

	var postCount int
	var attachCount int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)

		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive)

		CheckError(err, "error with replies replies db scan")

		post.Actor = actor.Id

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)				

		result = append(result, post)			
	}

	nColl.OrderedItems = result	

	return &nColl, postCount, attachCount
}

func CheckIfObjectOP(db *sql.DB, id string) bool {

	var count int
	
	query := `select count(id) from replies where inreplyto='' and id=$1 `

	rows, err := db.Query(query, id)
	CheckError(err, "error checking if ID is OP")

	defer rows.Close()
	rows.Next()
	rows.Scan(&count)

	if count > 0 {
		return true
	}

	return false
}

func GetObjectRepliesCount(db *sql.DB, parent ObjectBase) (int, int) {

	var countId int
	var countImg int

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over() from (select id, attachment from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select id, attachment from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x`
	
	rows, err := db.Query(query, parent.Id)	
	
	CheckError(err, "error with replies count db query")

	defer rows.Close()
	rows.Next()
	rows.Scan(&countId, &countImg)

	return countId, countImg
}

func GetObjectAttachment(db *sql.DB, id string) []ObjectBase {

	var attachments []ObjectBase	

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`

	rows, err := db.Query(query,  id)	

	CheckError(err, "could not select object attachment query")

	defer rows.Close()
	for rows.Next() {
		var attachment = new(ObjectBase)

		err = rows.Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published)
		if err !=nil{
			fmt.Println("error with attachment db query")
			panic(err)
		}

		attachments = append(attachments, *attachment)
	}

	return attachments
}

func GetObjectPreview(db *sql.DB, id string) *NestedObjectBase {

	var preview NestedObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`
	
	rows, err := db.Query(query, id)	

	CheckError(err, "could not select object preview query")	

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published)
	}

	return &preview
}

func GetObjectPostsTotalDB(db *sql.DB, actor Actor) int{

	count := 0
	query := `select count(id) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note')`

	rows, err := db.Query(query, actor.Id)	

	CheckError(err, "could not select post total count query")		

	defer rows.Close()	
	for rows.Next() {
		err = rows.Scan(&count)
		CheckError(err, "error with total post db scan")
	}
	
	return count
}

func GetObjectImgsTotalDB(db *sql.DB, actor Actor) int{

	count := 0
	query := `select count(attachment) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note' )`

	rows, err := db.Query(query, actor.Id)	

	CheckError(err, "error with posts total db query")			

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&count)

		CheckError(err, "error with total post db scan")
	}
	
	return count
}

func DeletePreviewFromFile(db *sql.DB, id string) {

	var query = `select href from activitystream where id in (select preview from activitystream where id=$1)`

	rows, err := db.Query(query, id)	

	CheckError(err, "error query delete attachment")				

	defer rows.Close()
	for rows.Next() {
		var href string

		err := rows.Scan(&href)
		href = strings.Replace(href, Domain + "/", "", 1)
		CheckError(err, "error scanning delete attachment")

		if(href != "static/notfound.png") {
			_, err = os.Stat(href)
			if err == nil {
				os.Remove(href)
			}
		}
	}

}

func RemovePreviewFromFile(db *sql.DB, id string) {

	var query = `select href from activitystream where id in (select preview from activitystream where id=$1)`

	rows, err := db.Query(query, id)	

	CheckError(err, "error query delete attachment")				

	defer rows.Close()
	for rows.Next() {
		var href string

		err := rows.Scan(&href)
		href = strings.Replace(href, Domain + "/", "", 1)
		CheckError(err, "error scanning delete attachment")

		if(href != "static/notfound.png") {
			_, err = os.Stat(href)
			if err == nil {
				os.Remove(href)
			}
		}
	}

	DeletePreviewFromDB(db, id)
}

func DeleteAttachmentFromFile(db *sql.DB, id string) {

	var query = `select href from activitystream where id in (select attachment from activitystream where id=$1)`

	rows, err := db.Query(query, id)	

	CheckError(err, "error query delete attachment")				

	defer rows.Close()
	for rows.Next() {
		var href string

		err := rows.Scan(&href)
		href = strings.Replace(href, Domain + "/", "", 1)
		CheckError(err, "error scanning delete preview")

		if(href != "static/notfound.png") {
			_, err = os.Stat(href)
			if err == nil {
				os.Remove(href)
			}
		}
	}

}

func TombstonePreviewRepliesFromDB(db *sql.DB, id string) {
	var query = `select id from activitystream where id in (select id from replies where inreplyto=$1)`
	
	rows, err := db.Query(query, id)

	CheckError(err, "error query tombstone preview replies")

	defer rows.Close()	
	for rows.Next() {
		var attachment string

		err := rows.Scan(&attachment)

		CheckError(err, "error scanning tombstone preview")
		
		DeletePreviewFromFile(db, attachment)
		TombstonePreviewFromDB(db, attachment)
	}	
}

func TombstoneAttachmentRepliesFromDB(db *sql.DB, id string) {
	var query = `select id from activitystream where id in (select id from replies where inreplyto=$1)`
	
	rows, err := db.Query(query, id)	

	CheckError(err, "error query tombstone attachment replies")

	defer rows.Close()	
	for rows.Next() {
		var attachment string

		err := rows.Scan(&attachment)

		CheckError(err, "error scanning delete attachment")
		
		DeleteAttachmentFromFile(db, attachment)
		TombstoneAttachmentFromDB(db, attachment)
	}	
}

func TombstoneAttachmentFromDB(db *sql.DB, id string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from activitystream where id=$3)`

	_, err := db.Exec(query, Domain + "/static/notfound.png", datetime, id)	

	CheckError(err, "error with tombstone attachment")

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from cacheactivitystream where id=$3)`

	_, err = db.Exec(query, Domain + "/static/notfound.png", datetime, id)	

	CheckError(err, "error with tombstone cache attachment")		
}

func DeleteAttachmentFromDB(db *sql.DB, id string) {
	var query = `delete from activitystream where id in (select attachment from activitystream where id=$1)`

	_, err := db.Exec(query, id)	

	CheckError(err, "error with delete attachment")

	query = `delete from cacheactivitystream where id in (select attachment from cacheactivitystream where id=$1)`

	_, err = db.Exec(query, id)	

	CheckError(err, "error with delete cache attachment")		
}

func TombstonePreviewFromDB(db *sql.DB, id string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from activitystream where id=$3)`

	_, err := db.Exec(query, Domain + "/static/notfound.png", datetime, id)	

	CheckError(err, "error with tombstone preview")

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`

	_, err = db.Exec(query, Domain + "/static/notfound.png", datetime, id)	

	CheckError(err, "error with tombstone cache preview")		
}

func DeletePreviewFromDB(db *sql.DB, id string) {
	var query = `delete from activitystream  where id=$1`

	_, err := db.Exec(query, id)	

	CheckError(err, "error with delete preview")

	query = `delete from cacheactivitystream where id in (select preview from cacheactivitystream where id=$1)`

	_, err = db.Exec(query, id)	

	CheckError(err, "error with delete cache preview")		
}

func DeleteObjectRepliedTo(db *sql.DB, id string){
	query := `delete from replies where id=$1`
	_, err := db.Exec(query, id)

	CheckError(err, "error with delete object replies")	
}

func TombstoneObjectFromDB(db *sql.DB, id string) {
	datetime := time.Now().UTC().Format(time.RFC3339)
	var query = `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id=$2`

	_, err := db.Exec(query, datetime, id)	

	CheckError(err, "error with tombstone object")

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='',  deleted=$1 where id=$2`

	_, err = db.Exec(query, datetime, id)	

	CheckError(err, "error with tombstone cache object")	
}

func DeleteObjectFromDB(db *sql.DB, id string) {
 var query = `delete from activitystream where id=$1`

	_, err := db.Exec(query, id)	

	CheckError(err, "error with delete object")

	query = `delete from cacheactivitystream where id=$1`

	_, err = db.Exec(query, id)	

	CheckError(err, "error with delete cache object")	
}

func DeleteObjectsInReplyTo(db *sql.DB, id string) {
	query := `delete from replies where id in (select id from replies where inreplyto=$1)`	

	_, err := db.Exec(query, id)

	CheckError(err, "error with delete object replies to")		
}

func TombstoneObjectRepliesFromDB(db *sql.DB, id string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`

	_, err := db.Exec(query, datetime, id)	
	CheckError(err, "error with tombstone object replies")

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`

	_, err = db.Exec(query, datetime, id)	
	CheckError(err, "error with tombstone object cache replies")	

}

func SetAttachmentFromDB(db *sql.DB, id string, _type string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id=$3)`

	_, err := db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set attachment")

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream  where id=$3)`

	_, err = db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set cache attachment")		
}

func SetAttachmentRepliesFromDB(db *sql.DB, id string, _type string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id in (select id from replies where inreplyto=$3))`

	_, err := db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set attachment")

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream where id in (select id from replies where inreplyto=$3))`

	_, err = db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set cache attachment")		
}

func SetPreviewFromDB(db *sql.DB, id string, _type string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id=$3)`

	_, err := db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set preview")

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`

	_, err = db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set cache preview")		
}

func SetPreviewRepliesFromDB(db *sql.DB, id string, _type string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id in (select id from replies where inreplyto=$3))`

	_, err := db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set preview")

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id in (select id from replies where inreplyto=$3))`	

	_, err = db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set cache preview")		
}

func SetObjectFromDB(db *sql.DB, id string, _type string) {
	datetime := time.Now().UTC().Format(time.RFC3339)
	
	var query = `update activitystream set type=$1, deleted=$2 where id=$3`

	_, err := db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set object")

	query = `update cacheactivitystream set type=$1, deleted=$2 where id=$3`

	_, err = db.Exec(query, _type, datetime, id)	

	CheckError(err, "error with set cache object")	
}

func SetObjectRepliesFromDB(db *sql.DB, id string, _type string) {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	_, err := db.Exec(query, _type, datetime, id)	
	CheckError(err, "error with set object replies")

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	_, err = db.Exec(query, _type, datetime, id)	
	CheckError(err, "error with set cache object replies")	
}

func SetObject(db *sql.DB, id string, _type string) {
	SetAttachmentFromDB(db, id, _type);
	SetPreviewFromDB(db, id, _type);
	SetObjectFromDB(db, id, _type);
}

func SetObjectAndReplies(db *sql.DB, id string, _type string) {
	SetAttachmentFromDB(db, id, _type);
	SetPreviewFromDB(db, id, _type);
	SetObjectRepliesFromDB(db, id, _type);
	SetAttachmentRepliesFromDB(db, id, _type);
	SetPreviewRepliesFromDB(db, id, _type);
	SetObjectFromDB(db, id, _type);	
}

func DeleteObject(db *sql.DB, id string) {
	DeleteReportActivity(db, id)
	DeleteAttachmentFromFile(db, id)
	DeleteAttachmentFromDB(db, id)
	DeletePreviewFromFile(db, id)
	DeletePreviewFromDB(db, id)	
	DeleteObjectFromDB(db, id)
	DeleteObjectRepliedTo(db, id)
}

func TombstoneObject(db *sql.DB, id string) {
	DeleteReportActivity(db, id)
	DeleteAttachmentFromFile(db, id)
	TombstoneAttachmentFromDB(db, id)
	DeletePreviewFromFile(db, id)
	TombstonePreviewFromDB(db, id)	
	TombstoneObjectFromDB(db, id)
}

func TombstoneObjectAndReplies(db *sql.DB, id string) {
	DeleteReportActivity(db, id)	
	DeleteAttachmentFromFile(db, id)
	TombstoneAttachmentFromDB(db, id)	
	DeletePreviewFromFile(db, id)
	TombstonePreviewFromDB(db, id)		
	TombstoneObjectRepliesFromDB(db, id)
	TombstoneAttachmentRepliesFromDB(db, id)
	TombstonePreviewRepliesFromDB(db, id)
	TombstoneObjectFromDB(db, id)
}

func GetRandomCaptcha(db *sql.DB) string{
	query := `select identifier from verification where type='captcha' order by random() limit 1`

	rows, err := db.Query(query)

	CheckError(err, "could not get captcha")

	var verify string

	defer rows.Close()
	
	rows.Next()
	err = rows.Scan(&verify)
	
	CheckError(err, "Could not get verify captcha")

	return verify
}

func GetCaptchaTotal(db *sql.DB) int{
	query := `select count(*) from verification where type='captcha'`

	rows, err := db.Query(query)
	
	CheckError(err, "could not get query captcha total")	

	defer rows.Close()
	
	var count int
	for rows.Next(){
		if err := rows.Scan(&count); err != nil{
			CheckError(err, "could not get captcha total")
		}
	}

	return count
}

func GetCaptchaCodeDB(db *sql.DB, verify string) string {

	query := `select code from verification where identifier=$1 limit 1`

	rows, err := db.Query(query, verify)

	CheckError(err, "could not get captcha verifciation")

	defer rows.Close()

	var code string
	
	rows.Next()
	err = rows.Scan(&code)

	if err != nil {
		fmt.Println("Could not get verification captcha")
	}

	return code
}

func GetActorAuth(db *sql.DB, actor string) []string {
	query := `select type from actorauth where board=$1`

	rows, err := db.Query(query, actor)	

	CheckError(err, "could not get actor auth")	

	defer rows.Close()	

	var auth []string
	
	for rows.Next() {
		var e string
		err = rows.Scan(&e)

		CheckError(err, "could not get actor auth row scan")		

		auth = append(auth, e)
	}

	return auth
}

func DeleteCaptchaCodeDB(db *sql.DB, verify string) {
	query := `delete from verification where identifier=$1`

	_, err := db.Exec(query, verify)	

	CheckError(err, "could not delete captcah code db")

	os.Remove("./" + verify)
}

func EscapeString(text string) string {
	text = strings.Replace(text, "<", "&lt;", -1)
	return text
}

func GetActorReportedTotal(db *sql.DB, id string) int {
	query := `select count(id) from reported where board=$1`

	rows, err := db.Query(query, id)	

	CheckError(err, "error getting actor reported total query")

	defer rows.Close()

	var count int
	for rows.Next() {
		rows.Scan(&count)
	}
	
	return count
}

func GetActorReportedDB(db *sql.DB, id string) []ObjectBase {
	var nObj []ObjectBase

	query := `select id, count from reported where board=$1`

	rows, err := db.Query(query, id)	

	CheckError(err, "error getting actor reported query")

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		rows.Scan(&obj.Id, &obj.Size)

		nObj = append(nObj, obj)
	}

	return nObj
}

func GetActorPemFromDB(db *sql.DB, pemID string) PublicKeyPem {
	query := `select id, owner, file from publickeypem where id=$1`
	rows, err := db.Query(query, pemID)

	CheckError(err, "could not get public key pem from database")

	var pem PublicKeyPem
	
	defer rows.Close()
	rows.Next()
	rows.Scan(&pem.Id, &pem.Owner, &pem.PublicKeyPem)
	f, _ := os.ReadFile(pem.PublicKeyPem)

	pem.PublicKeyPem = strings.ReplaceAll(string(f), "\r\n", `\n`)
	
	return pem
}

func MarkObjectSensitive(db *sql.DB, id string, sensitive bool) {
	var query = `update activitystream set sensitive=$1 where id=$2`
	_, err := db.Exec(query, sensitive, id)

	CheckError(err, "error updating sensitive object in activitystream")

	query = `update cacheactivitystream set sensitive=$1 where id=$2`
	_, err = db.Exec(query, sensitive, id)

	CheckError(err, "error updating sensitive object in cacheactivitystream")	
}
