package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func startServer() {
	var err error
	db, err = sql.Open("sqlite3", "forum.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	initDB()

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Custom mux for 404 support
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/post/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			render405(w, r)
			return
		}
		postDetailHandler(w, r)
	})
	mux.HandleFunc("/edit_comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			render405(w, r)
			return
		}
		editCommentHandler(w, r)
	})
	mux.HandleFunc("/delete_comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			render405(w, r)
			return
		}
		deleteCommentHandler(w, r)
	})
	mux.HandleFunc("/like_comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			render405(w, r)
			return
		}
		likeCommentHandler(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			if r.Method != http.MethodGet {
				render405(w, r)
				return
			}
			homeHandler(w, r)
		case "/register":
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				render405(w, r)
				return
			}
			registerHandler(w, r)
		case "/login":
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				render405(w, r)
				return
			}
			loginHandler(w, r)
		case "/logout":
			if r.Method != http.MethodGet {
				render405(w, r)
				return
			}
			logoutHandler(w, r)
		case "/create_post":
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				render405(w, r)
				return
			}
			if r.Method == http.MethodGet && len(r.URL.Query()) > 0 {
				render405(w, r)
				return
			}
			createPostHandler(w, r)
		case "/like_post":
			if r.Method != http.MethodGet {
				render405(w, r)
				return
			}
			likePostHandler(w, r)
		case "/edit_post":
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				render405(w, r)
				return
			}
			editPostHandler(w, r)
		case "/delete_post":
			if r.Method != http.MethodGet {
				render405(w, r)
				return
			}
			deletePostHandler(w, r)
		default:
			render404(w, r)
		}
	})
	// Детальная страница поста

	fmt.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// Centralized error rendering helpers
func render400(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": message})
}

// Centralized 405 error rendering
func render405(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Method Not Allowed (405)"})
}

func render500(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusInternalServerError)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": message})
}

// Centralized 404 error rendering
func render404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Page not found (404)"})
}

// Обработчик редактирования поста
func editPostHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	postID := r.URL.Query().Get("id")
	if postID == "" {
		render400(w, "Post not found.")
		return
	}
	// Проверяем, что пользователь — автор
	var uid int
	err := db.QueryRow("SELECT user_id FROM posts WHERE id = ?", postID).Scan(&uid)
	if err != nil || uid != user.ID {
		render400(w, "Access denied or post not found.")
		return
	}
	if r.Method == http.MethodGet {
		var title, content string
		err := db.QueryRow("SELECT title, content FROM posts WHERE id = ?", postID).Scan(&title, &content)
		if err != nil {
			render400(w, "Post not found.")
			return
		}
		data := map[string]interface{}{"ID": postID, "Title": title, "Content": content}
		templates.ExecuteTemplate(w, "edit_post.html", data)
		return
	}
	if r.Method == http.MethodPost {
		title := r.FormValue("title")
		content := r.FormValue("content")
		if title == "" || content == "" {
			render400(w, "All fields are required.")
			return
		}
		if len(title) > 20 {
			render400(w, "Title must be 20 characters or less.")
			return
		}
		_, err := db.Exec("UPDATE posts SET title = ?, content = ? WHERE id = ?", title, content, postID)
		if err != nil {
			render500(w, "Error updating post.")
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

var db *sql.DB
var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"iterate": func(count int) []int {
		s := make([]int, count)
		for i := 0; i < count; i++ {
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
	"inc": func(a int) int { return a + 1 },
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
		} else {
			if len(params) > 0 && params[len(params)-1] == '&' {
				params = params[:len(params)-1]
			}
		}
		if params == "" {
			return "/"
		}
		if params[len(params)-1] == '&' {
			params = params[:len(params)-1]
		}
		return "/?" + params
	},
}).ParseGlob("templates/*.html"))

// Структура для сессии
// ...existing code...

type User struct {
	ID       int
	Email    string
	Username string
	Password string
}

type Category struct {
	ID   int
	Name string
}

type Post struct {
	ID         int
	UserID     int
	Title      string
	Content    string
	CreatedAt  string
	Author     string
	Likes      int
	Dislikes   int
	Categories []Category // New: categories for this post
}

type Comment struct {
	ID        int
	PostID    int
	UserID    int
	Content   string
	CreatedAt string
	Author    string
	Likes     int
	Dislikes  int
}

type Session struct {
	ID      int
	UserID  int
	UUID    string
	Expires time.Time
}

// Редактирование комментария
func editCommentHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	commentID := r.URL.Query().Get("id")
	postID := r.URL.Query().Get("post")
	if commentID == "" || postID == "" {
		render404(w, r)
		return
	}
	// Проверка авторства
	var uid int
	err := db.QueryRow("SELECT user_id FROM comments WHERE id = ?", commentID).Scan(&uid)
	if err != nil || uid != user.ID {
		render400(w, "Access denied or comment not found.")
		return
	}
	if r.Method == http.MethodGet {
		var content string
		err := db.QueryRow("SELECT content FROM comments WHERE id = ?", commentID).Scan(&content)
		if err != nil {
			render400(w, "Comment not found.")
			return
		}
		data := map[string]interface{}{"Content": content, "ID": commentID, "PostID": postID}
		templates.ExecuteTemplate(w, "edit_comment.html", data)
		return
	}
	if r.Method == http.MethodPost {
		content := r.FormValue("content")
		if content == "" || len(content) > 500 {
			render400(w, "Comment must be 1-500 chars.")
			return
		}
		_, err := db.Exec("UPDATE comments SET content = ? WHERE id = ?", content, commentID)
		if err != nil {
			render500(w, "Error updating comment.")
			return
		}
		http.Redirect(w, r, "/post/"+postID, http.StatusSeeOther)
	}
}

// Удаление комментария
func deleteCommentHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	commentID := r.URL.Query().Get("id")
	postID := r.URL.Query().Get("post")
	if commentID == "" || postID == "" {
		render404(w, r)
		return
	}
	var uid int
	err := db.QueryRow("SELECT user_id FROM comments WHERE id = ?", commentID).Scan(&uid)
	if err != nil || uid != user.ID {
		render400(w, "Access denied or comment not found.")
		return
	}
	db.Exec("DELETE FROM comment_likes WHERE comment_id = ?", commentID)
	db.Exec("DELETE FROM comments WHERE id = ?", commentID)
	http.Redirect(w, r, "/post/"+postID, http.StatusSeeOther)
}

// Лайк/дизлайк комментария
func likeCommentHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	commentID := r.URL.Query().Get("id")
	postID := r.URL.Query().Get("post")
	if commentID == "" || postID == "" {
		render404(w, r)
		return
	}
	isLike := r.URL.Query().Get("like") == "1"
	var existingLike bool
	err := db.QueryRow("SELECT is_like FROM comment_likes WHERE comment_id = ? AND user_id = ?", commentID, user.ID).Scan(&existingLike)
	if err == sql.ErrNoRows {
		// Нет лайка/дизлайка — добавить
		_, err = db.Exec("INSERT INTO comment_likes (comment_id, user_id, is_like) VALUES (?, ?, ?)", commentID, user.ID, isLike)
	} else if err == nil {
		if existingLike == isLike {
			// Повторное действие — удалить
			_, err = db.Exec("DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?", commentID, user.ID)
		} else {
			// Смена лайк/дизлайк
			_, err = db.Exec("UPDATE comment_likes SET is_like = ? WHERE comment_id = ? AND user_id = ?", isLike, commentID, user.ID)
		}
	}
	http.Redirect(w, r, "/post/"+postID, http.StatusSeeOther)
}

// Детальная страница поста
func postDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Ожидаем /post/{id}
	id := r.URL.Path[len("/post/"):]
	if id == "" {
		render404(w, r)
		return
	}
	user := getCurrentUser(r)
	// POST: добавление комментария
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
		_, err := db.Exec("INSERT INTO comments (post_id, user_id, content, created_at) VALUES (?, ?, ?, ?)", id, user.ID, content, time.Now())
		if err != nil {
			templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Error saving comment."})
			return
		}
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	// Получаем пост
	var p Post
	var createdAt string
	err := db.QueryRow(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id WHERE posts.id = ?`, id).Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &createdAt, &p.Author)
	if err != nil {
		render404(w, r)
		return
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	if err == nil {
		p.CreatedAt = t.Format("02.01.2006 15:04")
	} else {
		p.CreatedAt = createdAt
	}
	db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 1", p.ID).Scan(&p.Likes)
	db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 0", p.ID).Scan(&p.Dislikes)
	// Категории поста
	catRows, _ := db.Query("SELECT categories.id, categories.name FROM categories JOIN post_categories ON categories.id = post_categories.category_id WHERE post_categories.post_id = ?", p.ID)
	var cats []Category
	for catRows.Next() {
		var c Category
		catRows.Scan(&c.ID, &c.Name)
		cats = append(cats, c)
	}
	catRows.Close()
	p.Categories = cats

	// Комментарии
	comments := []Comment{}
	rows, _ := db.Query(`SELECT comments.id, comments.post_id, comments.user_id, comments.content, comments.created_at, users.username FROM comments JOIN users ON comments.user_id = users.id WHERE comments.post_id = ? ORDER BY comments.created_at ASC`, p.ID)
	for rows.Next() {
		var c Comment
		var cAt string
		rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &cAt, &c.Author)
		t, err := time.Parse(time.RFC3339, cAt)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", cAt)
		}
		if err == nil {
			c.CreatedAt = t.Format("02.01.2006 15:04")
		} else {
			c.CreatedAt = cAt
		}
		// Лайки/дизлайки для комментария
		db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 1", c.ID).Scan(&c.Likes)
		db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 0", c.ID).Scan(&c.Dislikes)
		comments = append(comments, c)
	}
	rows.Close()

	data := map[string]interface{}{
		"Post":     p,
		"Comments": comments,
		"User":     user,
	}
	templates.ExecuteTemplate(w, "post_detail.html", data)
}

// --- Handlers and helper functions moved from main.go ---

func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	postID := r.URL.Query().Get("id")
	if postID == "" {
		templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Post not found."})
		return
	}
	// Проверяем, что пользователь — автор
	var uid int
	err := db.QueryRow("SELECT user_id FROM posts WHERE id = ?", postID).Scan(&uid)
	if err != nil || uid != user.ID {
		templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Access denied or post not found."})
		return
	}
	// Удаляем пост и связанные данные
	db.Exec("DELETE FROM post_categories WHERE post_id = ?", postID)
	db.Exec("DELETE FROM post_likes WHERE post_id = ?", postID)
	db.Exec("DELETE FROM comments WHERE post_id = ?", postID)
	db.Exec("DELETE FROM posts WHERE id = ?", postID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func initDB() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		username TEXT NOT NULL,
		password TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);
	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		title TEXT,
		content TEXT,
		created_at DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS post_categories (
		post_id INTEGER,
		category_id INTEGER,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(category_id) REFERENCES categories(id)
	);
	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER,
		user_id INTEGER,
		content TEXT,
		created_at DATETIME,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS post_likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER,
		user_id INTEGER,
		is_like BOOLEAN,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS comment_likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		comment_id INTEGER,
		user_id INTEGER,
		is_like BOOLEAN,
		FOREIGN KEY(comment_id) REFERENCES comments(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	`)
	if err != nil {
		log.Fatal(err)
	}
	// Добавить базовые категории, если их нет
	var count int
	db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count)
	if count == 0 {
		db.Exec("INSERT INTO categories (name) VALUES (?), (?), (?)", "General", "Programming", "Offtopic")
	}
	// Создание таблицы сессий
	db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		uuid TEXT UNIQUE,
		expires DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);`)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)

	// Получаем категории
	categories := []Category{}
	rows, _ := db.Query("SELECT id, name FROM categories")
	for rows.Next() {
		var c Category
		rows.Scan(&c.ID, &c.Name)
		categories = append(categories, c)
	}
	rows.Close()

	// Параметры пагинации
	const pageSize = 5
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
		if page < 1 {
			page = 1
		}
	}
	offset := (page - 1) * pageSize

	// Для шаблона: текущая категория и фильтр
	currentCategory := r.URL.Query().Get("category")
	currentFilter := r.URL.Query().Get("filter")

	// Фильтрация и подсчет общего количества постов
	var (
		posts      []Post
		rowsPosts  *sql.Rows
		totalPosts int
		countQuery string
		args       []interface{}
	)
	filter := r.URL.Query().Get("filter")
	if filter == "myposts" && user != nil {
		countQuery = "SELECT COUNT(*) FROM posts WHERE user_id = ?"
		args = append(args, user.ID)
		db.QueryRow(countQuery, args...).Scan(&totalPosts)
		rowsPosts, _ = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id WHERE posts.user_id = ? ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, user.ID, pageSize, offset)
	} else if filter == "liked" && user != nil {
		countQuery = "SELECT COUNT(DISTINCT posts.id) FROM posts JOIN post_likes ON posts.id = post_likes.post_id WHERE post_likes.user_id = ? AND post_likes.is_like = 1"
		args = append(args, user.ID)
		db.QueryRow(countQuery, args...).Scan(&totalPosts)
		rowsPosts, _ = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id JOIN post_likes ON posts.id = post_likes.post_id WHERE post_likes.user_id = ? AND post_likes.is_like = 1 GROUP BY posts.id ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, user.ID, pageSize, offset)
	} else if cat := r.URL.Query().Get("category"); cat != "" {
		countQuery = "SELECT COUNT(*) FROM post_categories WHERE category_id = ?"
		args = append(args, cat)
		db.QueryRow(countQuery, args...).Scan(&totalPosts)
		rowsPosts, _ = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id JOIN post_categories ON posts.id = post_categories.post_id WHERE post_categories.category_id = ? ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, cat, pageSize, offset)
	} else {
		countQuery = "SELECT COUNT(*) FROM posts"
		db.QueryRow(countQuery).Scan(&totalPosts)
		rowsPosts, _ = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, pageSize, offset)
	}
	for rowsPosts.Next() {
		var p Post
		rowsPosts.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author)
		// Форматируем дату в "02.01.2006 15:04"
		t, err := time.Parse(time.RFC3339, p.CreatedAt)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", p.CreatedAt)
		}
		if err == nil {
			p.CreatedAt = t.Format("02.01.2006 15:04")
		}
		// Лайки/дизлайки
		db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 1", p.ID).Scan(&p.Likes)
		db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 0", p.ID).Scan(&p.Dislikes)
		// Получаем категории поста
		catRows, _ := db.Query("SELECT categories.id, categories.name FROM categories JOIN post_categories ON categories.id = post_categories.category_id WHERE post_categories.post_id = ?", p.ID)
		var cats []Category
		for catRows.Next() {
			var c Category
			catRows.Scan(&c.ID, &c.Name)
			cats = append(cats, c)
		}
		catRows.Close()
		p.Categories = cats
		posts = append(posts, p)
	}
	rowsPosts.Close()

	// Параметры для пагинации
	totalPages := (totalPosts + pageSize - 1) / pageSize

	data := map[string]interface{}{
		"User":              user,
		"Categories":        categories,
		"Posts":             posts,
		"Page":              page,
		"TotalPages":        totalPages,
		"CurrentCategoryID": currentCategory,
		"CurrentFilter":     currentFilter,
	}
	templates.ExecuteTemplate(w, "home_page.html", data)
}

func getCurrentUser(r *http.Request) *User {
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value == "" {
		return nil
	}
	// В реальном проекте — искать по сессии, здесь — по email для простоты
	return getUserBySession(cookie.Value)
}

func getUserBySession(session string) *User {
	// Ищем сессию в БД
	var userID int
	var expires time.Time
	err := db.QueryRow("SELECT user_id, expires FROM sessions WHERE uuid = ?", session).Scan(&userID, &expires)
	if err != nil || time.Now().After(expires) {
		return nil
	}
	var u User
	err = db.QueryRow("SELECT id, email, username, password FROM users WHERE id = ?", userID).Scan(&u.ID, &u.Email, &u.Username, &u.Password)
	if err != nil {
		return nil
	}
	return &u
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		if len(q) > 0 {
			render405(w, r)
			return
		}
		categories := []Category{}
		rows, _ := db.Query("SELECT id, name FROM categories")
		for rows.Next() {
			var c Category
			rows.Scan(&c.ID, &c.Name)
			categories = append(categories, c)
		}
		rows.Close()
		data := map[string]interface{}{"Categories": categories}
		templates.ExecuteTemplate(w, "create_post.html", data)
		return
	}
	if r.Method == http.MethodPost {
		r.ParseForm()
		title := strings.TrimSpace(r.Form.Get("title"))
		content := strings.TrimSpace(r.Form.Get("content"))
		catIDs := r.Form["categories"]
		emptyCat := false
		for _, cid := range catIDs {
			if strings.TrimSpace(cid) == "" {
				emptyCat = true
				break
			}
		}

		// Проверка: если параметры пустые и в r.Form, и в r.URL.Query, возвращаем 400
		q := r.URL.Query()
		qTitle := strings.TrimSpace(q.Get("title"))
		qContent := strings.TrimSpace(q.Get("content"))
		qCatIDs := q["categories"]
		emptyQCat := false
		for _, cid := range qCatIDs {
			if strings.TrimSpace(cid) == "" {
				emptyQCat = true
				break
			}
		}
		if (title == "" && qTitle == "") || (content == "" && qContent == "") || (len(catIDs) == 0 && len(qCatIDs) == 0) || emptyCat || emptyQCat {
			render400(w, "All fields and at least one category are required.")
			return
		}
		if len(title) > 20 {
			render400(w, "Title must be 20 characters or less.")
			return
		}
		res, err := db.Exec("INSERT INTO posts (user_id, title, content, created_at) VALUES (?, ?, ?, ?)", user.ID, title, content, time.Now())
		if err != nil {
			render500(w, "Error creating post.")
			return
		}
		postID, _ := res.LastInsertId()
		for _, cid := range catIDs {
			db.Exec("INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)", postID, cid)
		}
		// После создания поста всегда перенаправляем на главную страницу
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func likePostHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	postID := r.URL.Query().Get("id")
	if postID == "" {
		templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Post not found."})
		return
	}
	isLike := r.URL.Query().Get("like") == "1"
	// Toggle like/dislike logic
	var existingLike bool
	err := db.QueryRow("SELECT is_like FROM post_likes WHERE post_id = ? AND user_id = ?", postID, user.ID).Scan(&existingLike)
	if err == sql.ErrNoRows {
		// No previous like/dislike, insert new
		_, err = db.Exec("INSERT INTO post_likes (post_id, user_id, is_like) VALUES (?, ?, ?)", postID, user.ID, isLike)
		if err != nil {
			templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Error updating like/dislike."})
			return
		}
	} else if err == nil {
		if existingLike == isLike {
			// Same action, remove like/dislike
			_, err = db.Exec("DELETE FROM post_likes WHERE post_id = ? AND user_id = ?", postID, user.ID)
			if err != nil {
				templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Error updating like/dislike."})
				return
			}
		} else {
			// Switch like/dislike
			_, err = db.Exec("UPDATE post_likes SET is_like = ? WHERE post_id = ? AND user_id = ?", isLike, postID, user.ID)
			if err != nil {
				templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Error updating like/dislike."})
				return
			}
		}
	} else {
		templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Error updating like/dislike."})
		return
	}
	// Redirect back to the page with all original query params (except id/like)
	referer := r.Header.Get("Referer")
	if referer != "" {
		// Remove anchor if present
		if idx := len(referer); idx > 0 {
			if hash := string(referer[len(referer)-1]); hash == "#" {
				referer = referer[:len(referer)-1]
			}
		}
		http.Redirect(w, r, referer, http.StatusSeeOther)
		return
	}
	q := r.URL.Query()
	q.Del("id")
	q.Del("like")
	params := q.Encode()
	redirectURL := "/"
	if params != "" {
		redirectURL += "?" + params
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		templates.ExecuteTemplate(w, "register.html", nil)
		return
	}
	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		username := r.FormValue("username")
		password := r.FormValue("password")
		if email == "" || username == "" || password == "" {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "All fields required"})
			return
		}
		var emailExists, usernameExists int
		db.QueryRow("SELECT COUNT(*) FROM users WHERE email = ?", email).Scan(&emailExists)
		db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&usernameExists)
		if emailExists > 0 {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Email already taken"})
			return
		}
		if usernameExists > 0 {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Username already taken"})
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := db.Exec("INSERT INTO users (email, username, password) VALUES (?, ?, ?)", email, username, string(hash))
		if err != nil {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Registration error"})
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		templates.ExecuteTemplate(w, "login.html", nil)
		return
	}
	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		password := r.FormValue("password")
		var id int
		var username, hash string
		err := db.QueryRow("SELECT id, username, password FROM users WHERE email = ?", email).Scan(&id, &username, &hash)
		if err != nil {
			templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Invalid credentials"})
			return
		}
		err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		if err != nil {
			templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Invalid credentials"})
			return
		}
		sid := uuid.New().String()
		expires := time.Now().Add(24 * time.Hour)
		// Удаляем старые сессии пользователя
		db.Exec("DELETE FROM sessions WHERE user_id = ?", id)
		// Добавляем новую сессию
		db.Exec("INSERT INTO sessions (user_id, uuid, expires) VALUES (?, ?, ?)", id, sid, expires)
		http.SetCookie(w, &http.Cookie{
			Name:    "session",
			Value:   sid,
			Expires: expires,
			Path:    "/",
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		db.Exec("DELETE FROM sessions WHERE uuid = ?", cookie.Value)
	}
	cookie = &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
