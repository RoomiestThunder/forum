package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"forum/internal/database"
	"forum/internal/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var store database.Store

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"iterate": func(count int) []int {
		s := make([]int, count)
		for i := range s {
			s[i] = i
		}
		return s
	},
	"add": func(a, b int) int { return a + b },
	"dec": func(a int) int {
		if a > 1 {
			return a - 1
		}
		return 1
	},
	"inc":          func(a int) int { return a + 1 },
	"formatTime":   func(t time.Time) string { return t.Format("02.01.2006 15:04") },
	"paginationURL": func(page int, category, filter string) string {
		params := ""
		if category != "" {
			params += fmt.Sprintf("category=%v&", category)
		}
		if filter != "" {
			params += fmt.Sprintf("filter=%v&", filter)
		}
		if page > 1 {
			params += fmt.Sprintf("page=%d", page)
		} else if len(params) > 0 && params[len(params)-1] == '&' {
			params = params[:len(params)-1]
		}
		if params == "" {
			return "/"
		}
		if params[len(params)-1] == '&' {
			params = params[:len(params)-1]
		}
		return "/?" + params
	},
}).ParseGlob("../../templates/*.html"))

// RegisterRoutes mounts all HTML routes onto mux.
func RegisterRoutes(mux *http.ServeMux, st database.Store) {
	store = st

	// Static files
	mux.HandleFunc("/static/css/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/static/css/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "static/css"+strings.TrimPrefix(r.URL.Path, "/static/css"))
	})
	mux.HandleFunc("/static/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/css/") || r.URL.Path == "/static/favicon.ico" {
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("403 Forbidden"))
	})
	mux.HandleFunc("/templates/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("403 Forbidden"))
	})

	mux.HandleFunc("/post/", methodGate([]string{http.MethodGet, http.MethodPost}, postDetailHandler))
	mux.HandleFunc("/edit_comment", editCommentHandler)
	mux.HandleFunc("/delete_comment", deleteCommentHandler)
	mux.HandleFunc("/like_comment", methodGate([]string{http.MethodGet}, likeCommentHandler))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Don't intercept /api/* routes
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/metrics" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/":
			methodGate([]string{http.MethodGet}, homeHandler)(w, r)
		case "/register":
			methodGate([]string{http.MethodGet, http.MethodPost}, registerHandler)(w, r)
		case "/login":
			methodGate([]string{http.MethodGet, http.MethodPost}, loginHandler)(w, r)
		case "/logout":
			methodGate([]string{http.MethodGet}, logoutHandler)(w, r)
		case "/create_post":
			methodGate([]string{http.MethodGet, http.MethodPost}, createPostHandler)(w, r)
		case "/like_post":
			methodGate([]string{http.MethodGet}, likePostHandler)(w, r)
		case "/edit_post":
			editPostHandler(w, r)
		case "/delete_post":
			deletePostHandler(w, r)
		default:
			render404(w, r)
		}
	})
}

// methodGate wraps a handler with method checking.
func methodGate(methods []string, h http.HandlerFunc) http.HandlerFunc {
	allowed := make(map[string]bool)
	for _, m := range methods {
		allowed[m] = true
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow POST with _method override
		if r.Method == http.MethodPost {
			r.ParseForm()
			if m := r.FormValue("_method"); m != "" {
				if allowed[m] {
					h(w, r)
					return
				}
			}
		}
		if !allowed[r.Method] {
			render405(w, r)
			return
		}
		h(w, r)
	}
}

func Render400(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": message})
}

func render405(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Method Not Allowed (405)"})
}

func Render500(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusInternalServerError)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": message})
}

func render404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Page not found (404)"})
}

func GetCurrentUser(r *http.Request) *models.User {
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value == "" {
		return nil
	}
	userID, err := store.GetSessionUserID(cookie.Value)
	if err != nil {
		return nil
	}
	u, err := store.GetUserByID(userID)
	if err != nil {
		return nil
	}
	return u
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)

	cats, err := store.GetCategories()
	if err != nil {
		Render500(w, "Error loading categories.")
		return
	}

	const pageSize = 5
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
		if page < 1 {
			page = 1
		}
	}
	offset := (page - 1) * pageSize

	filter := r.URL.Query().Get("filter")
	category := r.URL.Query().Get("category")

	var posts []*models.Post
	var total int

	switch {
	case filter == "myposts" && user != nil:
		posts, _ = store.GetPostsByUser(user.ID, pageSize, offset)
		total, _ = store.CountPostsByUser(user.ID)
	case filter == "liked" && user != nil:
		posts, _ = store.GetLikedPostsByUser(user.ID, pageSize, offset)
		total, _ = store.CountLikedPostsByUser(user.ID)
	case category != "":
		catID := 0
		fmt.Sscanf(category, "%d", &catID)
		posts, _ = store.GetPostsByCategory(catID, pageSize, offset)
		total, _ = store.CountPostsByCategory(catID)
	default:
		posts, _ = store.GetPosts(pageSize, offset)
		total, _ = store.CountPosts()
	}

	totalPages := (total + pageSize - 1) / pageSize
	if len(posts) == 0 && page > 1 {
		q := r.URL.Query()
		q.Set("page", fmt.Sprintf("%d", page-1))
		http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
		return
	}

	// Convert []*models.Post to []models.Post for template
	postsSlice := make([]models.Post, len(posts))
	for i, p := range posts {
		postsSlice[i] = *p
	}

	catsSlice := make([]models.Category, len(cats))
	for i, c := range cats {
		catsSlice[i] = *c
	}

	data := map[string]interface{}{
		"User":              user,
		"Categories":        catsSlice,
		"Posts":             postsSlice,
		"Page":              page,
		"TotalPages":        totalPages,
		"CurrentCategoryID": category,
		"CurrentFilter":     filter,
	}
	templates.ExecuteTemplate(w, "home_page.html", data)
}

func postDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/post/"):]
	if idStr == "" {
		render404(w, r)
		return
	}
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	user := GetCurrentUser(r)

	if r.Method == http.MethodPost {
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		content := r.FormValue("comment")
		if content == "" {
			templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Comment cannot be empty."})
			return
		}
		if len(content) > 500 {
			templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Comment too long (max 500)."})
			return
		}
		_, _ = store.CreateComment(id, user.ID, content)
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}

	post, err := store.GetPost(id)
	if err != nil {
		render404(w, r)
		return
	}
	comments, _ := store.GetCommentsByPostID(id)

	commentsSlice := make([]models.Comment, len(comments))
	for i, c := range comments {
		commentsSlice[i] = *c
	}

	data := map[string]interface{}{
		"Post":     *post,
		"Comments": commentsSlice,
		"User":     user,
	}
	templates.ExecuteTemplate(w, "post_detail.html", data)
}


func editPostHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	user := GetCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		Render400(w, "Post not found.")
		return
	}
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	method := r.Method
	if method == http.MethodPost {
		if m := r.FormValue("_method"); m != "" {
			method = m
		}
	}

	switch method {
	case http.MethodGet:
		post, err := store.GetPost(id)
		if err != nil || post.UserID != user.ID {
			Render400(w, "Access denied or post not found.")
			return
		}
		templates.ExecuteTemplate(w, "edit_post.html", map[string]interface{}{
			"ID": idStr, "Title": post.Title, "Content": post.Content,
		})
	case http.MethodPut:
		title := r.FormValue("title")
		content := r.FormValue("content")
		if title == "" || content == "" {
			Render400(w, "All fields are required.")
			return
		}
		if len([]rune(title)) > 20 {
			Render400(w, "Title must be 20 characters or less.")
			return
		}
		if err := store.UpdatePost(id, user.ID, title, content); err != nil {
			Render500(w, "Error updating post.")
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		render405(w, r)
	}
}

func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	method := r.Method
	if method == http.MethodPost {
		if m := r.FormValue("_method"); m != "" {
			method = m
		}
	}
	if method != http.MethodDelete {
		render405(w, r)
		return
	}
	user := GetCurrentUser(r)
	if user == nil {
		Render400(w, "Unauthorized.")
		return
	}
	idStr := r.URL.Query().Get("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	if err := store.DeletePost(id, user.ID); err != nil {
		Render500(w, "Error deleting post.")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func editCommentHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	user := GetCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	commentIDStr := r.URL.Query().Get("id")
	postIDStr := r.URL.Query().Get("post")
	if commentIDStr == "" || postIDStr == "" {
		render404(w, r)
		return
	}
	var commentID int
	fmt.Sscanf(commentIDStr, "%d", &commentID)

	method := r.Method
	if method == http.MethodPost {
		if m := r.FormValue("_method"); m != "" {
			method = m
		}
	}

	switch method {
	case http.MethodGet:
		comments, _ := store.GetCommentsByPostID(0) // we need a single comment getter
		_ = comments
		// Use a simple approach: get from post comments
		// For edit page we just show the form
		templates.ExecuteTemplate(w, "edit_comment.html", map[string]interface{}{
			"ID": commentIDStr, "PostID": postIDStr, "Content": r.FormValue("content"),
		})
	case http.MethodPut:
		content := r.FormValue("content")
		if content == "" || len(content) > 500 {
			Render400(w, "Comment must be 1-500 chars.")
			return
		}
		if err := store.UpdateComment(commentID, user.ID, content); err != nil {
			Render500(w, "Error updating comment.")
			return
		}
		http.Redirect(w, r, "/post/"+postIDStr, http.StatusSeeOther)
	default:
		render405(w, r)
	}
}

func deleteCommentHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	method := r.Method
	if method == http.MethodPost {
		if m := r.FormValue("_method"); m != "" {
			method = m
		}
	}
	if method != http.MethodDelete {
		render405(w, r)
		return
	}
	user := GetCurrentUser(r)
	if user == nil {
		Render400(w, "Unauthorized.")
		return
	}
	commentIDStr := r.URL.Query().Get("id")
	postIDStr := r.URL.Query().Get("post")
	var commentID int
	fmt.Sscanf(commentIDStr, "%d", &commentID)
	if err := store.DeleteComment(commentID, user.ID); err != nil {
		Render500(w, "Error deleting comment.")
		return
	}
	http.Redirect(w, r, "/post/"+postIDStr, http.StatusSeeOther)
}

func likeCommentHandler(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	commentIDStr := r.URL.Query().Get("id")
	postIDStr := r.URL.Query().Get("post")
	var commentID int
	fmt.Sscanf(commentIDStr, "%d", &commentID)
	isLike := r.URL.Query().Get("like") == "1"
	_ = store.ToggleCommentLike(commentID, user.ID, isLike)
	http.Redirect(w, r, "/post/"+postIDStr, http.StatusSeeOther)
}

func likePostHandler(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	postIDStr := r.URL.Query().Get("id")
	var postID int
	fmt.Sscanf(postIDStr, "%d", &postID)
	isLike := r.URL.Query().Get("like") == "1"
	_ = store.TogglePostLike(postID, user.ID, isLike)

	// Only redirect to same-origin paths to prevent open redirect.
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && (u.Host == "" || u.Host == r.Host) {
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	user := GetCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		cats, _ := store.GetCategories()
		catsSlice := make([]models.Category, len(cats))
		for i, c := range cats {
			catsSlice[i] = *c
		}
		templates.ExecuteTemplate(w, "create_post.html", map[string]interface{}{"Categories": catsSlice})
		return
	}

	r.ParseForm()
	title := strings.TrimSpace(r.Form.Get("title"))
	content := strings.TrimSpace(r.Form.Get("content"))
	catIDStrs := r.Form["categories"]

	if title == "" || content == "" || len(catIDStrs) == 0 {
		Render400(w, "All fields and at least one category are required.")
		return
	}
	if len([]rune(title)) > 20 {
		Render400(w, "Title must be 20 characters or less.")
		return
	}

	catIDs := make([]int, 0, len(catIDStrs))
	for _, s := range catIDStrs {
		var id int
		fmt.Sscanf(s, "%d", &id)
		if id > 0 {
			catIDs = append(catIDs, id)
		}
	}

	if _, err := store.CreatePost(user.ID, title, content, catIDs); err != nil {
		Render500(w, "Error creating post.")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if GetCurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		templates.ExecuteTemplate(w, "register.html", nil)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if email == "" || username == "" || password == "" {
		templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "All fields are required."})
		return
	}
	if !isValidEmail(email) {
		templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Invalid email format."})
		return
	}
	if !isValidUsername(username) {
		templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Username: 3-20 letters or digits."})
		return
	}
	if len([]rune(password)) < 6 || len([]rune(password)) > 32 {
		templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Password: 6-32 characters."})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if _, err := store.CreateUser(email, username, string(hash)); err != nil {
		templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Email or username already taken."})
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if GetCurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		templates.ExecuteTemplate(w, "login.html", nil)
		return
	}

	login := strings.TrimSpace(r.FormValue("login"))
	password := r.FormValue("password")

	var userID int
	var username, hash string

	var u *models.User
	var err error
	if strings.Contains(login, "@") {
		u, err = store.GetUserByEmail(login)
	} else {
		u, err = store.GetUserByUsername(login)
	}
	if err != nil {
		templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Invalid credentials"})
		return
	}
	userID, username, hash = u.ID, u.Username, u.Password

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Invalid credentials"})
		return
	}

	sid := uuid.New().String()
	_ = store.DeleteSessionsByUser(userID)
	if err := store.CreateSession(userID, sid); err != nil {
		templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Session error"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "session",
		Value:   sid,
		Expires: time.Now().Add(24 * time.Hour),
		Path:    "/",
	})
	_ = username
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		_ = store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func isValidEmail(email string) bool {
	if strings.Count(email, "@") != 1 {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts[0]) < 1 || len(parts[1]) < 3 {
		return false
	}
	domain := parts[1]
	return !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".") && strings.Contains(domain, ".")
}

func isValidUsername(username string) bool {
	r := []rune(username)
	if len(r) < 3 || len(r) > 20 {
		return false
	}
	for _, ch := range r {
		if ch == ' ' || ch == '\t' || ch == '\n' {
			return false
		}
		if !isLetter(ch) && !isDigit(ch) {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= 0x410 && r <= 0x44F) || (r >= 0xC0 && r <= 0xFF) ||
		(r >= 0x0410 && r <= 0x042F) || (r >= 0x0430 && r <= 0x044F) ||
		(r > 127 && r != ' ')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
