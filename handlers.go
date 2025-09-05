package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func startServer() {
	mux := http.NewServeMux()
	// Запретить доступ ко всему /static и его содержимому
	// Разрешить только /static/css/ и /static/favicon.ico, остальное запрещено
	// Запретить прямой доступ к /static/css и /static/css/
	mux.HandleFunc("/static/css", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/static/css/", func(w http.ResponseWriter, r *http.Request) {
		// Если это запрос к папке (без имени файла) — 404
		if r.URL.Path == "/static/css/" {
			http.NotFound(w, r)
			return
		}
		// Если это файл — отдаём
		http.ServeFile(w, r, "static/css"+strings.TrimPrefix(r.URL.Path, "/static/css"))
	})
	mux.HandleFunc("/static/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})
	mux.HandleFunc("/static", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 Forbidden"))
	})
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		// Запретить всё, кроме css и favicon
		if strings.HasPrefix(r.URL.Path, "/static/css/") || r.URL.Path == "/static/favicon.ico" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 Forbidden"))
	})
	var err error
	// Get database path from environment variable, fallback to "forum.db"
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "forum.db"
	}
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	initDB()

	mux.HandleFunc("/post/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			render405(w, r)
			return
		}
		postDetailHandler(w, r)
	})
	mux.HandleFunc("/templates", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 Forbidden"))
	})
	mux.HandleFunc("/templates/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 Forbidden"))
	})
	mux.HandleFunc("/edit_comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			editCommentHandler(w, r)
			return
		}
		r.ParseForm()
		if r.Method == http.MethodPut || (r.Method == http.MethodPost && r.FormValue("_method") == "PUT") {
			editCommentHandler(w, r)
			return
		}
		render405(w, r)
	})
	mux.HandleFunc("/delete_comment", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Method == http.MethodDelete || (r.Method == http.MethodPost && r.FormValue("_method") == "DELETE") {
			deleteCommentHandler(w, r)
			return
		}
		render405(w, r)
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
			if r.Method == http.MethodGet {
				editPostHandler(w, r)
				return
			}
			r.ParseForm()
			if r.Method == http.MethodPut || (r.Method == http.MethodPost && r.FormValue("_method") == "PUT") {
				editPostHandler(w, r)
				return
			}
			render405(w, r)
		case "/delete_post":
			r.ParseForm()
			if r.Method == http.MethodDelete || (r.Method == http.MethodPost && r.FormValue("_method") == "DELETE") {
				deletePostHandler(w, r)
				return
			}
			render405(w, r)
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
	r.ParseForm()
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
	method := r.Method
	if method == http.MethodPost && r.FormValue("_method") != "" {
		method = r.FormValue("_method")
	}
	if method == http.MethodGet {
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
	if method == http.MethodPut {
		title := r.FormValue("title")
		content := r.FormValue("content")
		if title == "" || content == "" {
			render400(w, "All fields are required.")
			return
		}
		if len([]rune(title)) > 20 {
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
	ID           int
	UserID       int
	Title        string
	Content      string
	CreatedAt    string
	Author       string
	Likes        int
	Dislikes     int
	Categories   []Category // New: categories for this post
	CommentCount int        // Количество комментариев
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
	r.ParseForm()
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
	method := r.Method
	if method == http.MethodPost && r.FormValue("_method") != "" {
		method = r.FormValue("_method")
	}
	if method == http.MethodGet {
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
	if method == http.MethodPut {
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
	r.ParseForm()
	method := r.Method
	if method == http.MethodPost && r.FormValue("_method") != "" {
		method = r.FormValue("_method")
	}
	if !(method == http.MethodDelete) {
		render405(w, r)
		return
	}
	user := getCurrentUser(r)
	if user == nil {
		render400(w, "Unauthorized.")
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
	if _, err := db.Exec("DELETE FROM comment_likes WHERE comment_id = ?", commentID); err != nil {
		render500(w, "Error deleting comment likes.")
		return
	}
	if _, err := db.Exec("DELETE FROM comments WHERE id = ?", commentID); err != nil {
		render500(w, "Error deleting comment.")
		return
	}
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
		if err != nil {
			render500(w, "Error adding like/dislike.")
			return
		}
	} else if err == nil {
		if existingLike == isLike {
			// Повторное действие — удалить
			_, err = db.Exec("DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?", commentID, user.ID)
			if err != nil {
				render500(w, "Error removing like/dislike.")
				return
			}
		} else {
			// Смена лайк/дизлайк
			_, err = db.Exec("UPDATE comment_likes SET is_like = ? WHERE comment_id = ? AND user_id = ?", isLike, commentID, user.ID)
			if err != nil {
				render500(w, "Error updating like/dislike.")
				return
			}
		}
	} else {
		render500(w, "Error checking existing like/dislike.")
		return
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
	// Лайки/дизлайки
	if err := db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 1", p.ID).Scan(&p.Likes); err != nil {
		p.Likes = 0 // Default value on error
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 0", p.ID).Scan(&p.Dislikes); err != nil {
		p.Dislikes = 0 // Default value on error
	}
	// Категории поста
	catRows, err := db.Query("SELECT categories.id, categories.name FROM categories JOIN post_categories ON categories.id = post_categories.category_id WHERE post_categories.post_id = ?", p.ID)
	if err != nil {
		render500(w, "Error loading post categories.")
		return
	}
	defer catRows.Close()
	var cats []Category
	for catRows.Next() {
		var c Category
		if err := catRows.Scan(&c.ID, &c.Name); err != nil {
			render500(w, "Error reading post categories.")
			return
		}
		cats = append(cats, c)
	}
	p.Categories = cats

	// Комментарии
	comments := []Comment{}
	rows, err := db.Query(`SELECT comments.id, comments.post_id, comments.user_id, comments.content, comments.created_at, users.username FROM comments JOIN users ON comments.user_id = users.id WHERE comments.post_id = ? ORDER BY comments.created_at ASC`, p.ID)
	if err != nil {
		render500(w, "Error loading comments.")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c Comment
		var cAt string
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &cAt, &c.Author); err != nil {
			render500(w, "Error reading comments.")
			return
		}
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
		if err := db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 1", c.ID).Scan(&c.Likes); err != nil {
			c.Likes = 0 // Default value on error
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 0", c.ID).Scan(&c.Dislikes); err != nil {
			c.Dislikes = 0 // Default value on error
		}
		comments = append(comments, c)
	}

	data := map[string]interface{}{
		"Post":     p,
		"Comments": comments,
		"User":     user,
	}
	templates.ExecuteTemplate(w, "post_detail.html", data)
}

// --- Handlers and helper functions moved from main.go ---

func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	method := r.Method
	if method == http.MethodPost && r.FormValue("_method") != "" {
		method = r.FormValue("_method")
	}
	if !(method == http.MethodDelete) {
		render405(w, r)
		return
	}
	user := getCurrentUser(r)
	if user == nil {
		render400(w, "Unauthorized.")
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
	if _, err := db.Exec("DELETE FROM post_categories WHERE post_id = ?", postID); err != nil {
		render500(w, "Error deleting post categories.")
		return
	}
	if _, err := db.Exec("DELETE FROM post_likes WHERE post_id = ?", postID); err != nil {
		render500(w, "Error deleting post likes.")
		return
	}
	if _, err := db.Exec("DELETE FROM comment_likes WHERE comment_id IN (SELECT id FROM comments WHERE post_id = ?)", postID); err != nil {
		render500(w, "Error deleting comment likes.")
		return
	}
	if _, err := db.Exec("DELETE FROM comments WHERE post_id = ?", postID); err != nil {
		render500(w, "Error deleting comments.")
		return
	}
	if _, err := db.Exec("DELETE FROM posts WHERE id = ?", postID); err != nil {
		render500(w, "Error deleting post.")
		return
	}
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
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		log.Fatal("Error checking categories count:", err)
	}
	if count == 0 {
		if _, err := db.Exec("INSERT INTO categories (name) VALUES (?), (?), (?)", "General", "Programming", "Offtopic"); err != nil {
			log.Fatal("Error inserting default categories:", err)
		}
	}
	// Создание таблицы сессий
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		uuid TEXT UNIQUE,
		expires DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);`); err != nil {
		log.Fatal("Error creating sessions table:", err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	user := getCurrentUser(r)

	// Получаем категории
	categories := []Category{}
	rows, err := db.Query("SELECT id, name FROM categories")
	if err != nil {
		render500(w, "Error loading categories.")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			render500(w, "Error reading categories.")
			return
		}
		categories = append(categories, c)
	}

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
		if err := db.QueryRow(countQuery, args...).Scan(&totalPosts); err != nil {
			render500(w, "Error counting posts.")
			return
		}
		rowsPosts, err = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id WHERE posts.user_id = ? ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, user.ID, pageSize, offset)
		if err != nil {
			render500(w, "Error loading posts.")
			return
		}
	} else if filter == "liked" && user != nil {
		countQuery = "SELECT COUNT(DISTINCT posts.id) FROM posts JOIN post_likes ON posts.id = post_likes.post_id WHERE post_likes.user_id = ? AND post_likes.is_like = 1"
		args = append(args, user.ID)
		if err := db.QueryRow(countQuery, args...).Scan(&totalPosts); err != nil {
			render500(w, "Error counting liked posts.")
			return
		}
		rowsPosts, err = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id JOIN post_likes ON posts.id = post_likes.post_id WHERE post_likes.user_id = ? AND post_likes.is_like = 1 GROUP BY posts.id ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, user.ID, pageSize, offset)
		if err != nil {
			render500(w, "Error loading liked posts.")
			return
		}
	} else if cat := r.URL.Query().Get("category"); cat != "" {
		// Проверяем, существует ли категория
		var catExists int
		if err := db.QueryRow("SELECT COUNT(*) FROM categories WHERE id = ?", cat).Scan(&catExists); err != nil {
			render500(w, "Error checking category.")
			return
		}
		if catExists == 0 {
			render404(w, r)
			return
		}
		countQuery = "SELECT COUNT(*) FROM post_categories WHERE category_id = ?"
		args = append(args, cat)
		if err := db.QueryRow(countQuery, args...).Scan(&totalPosts); err != nil {
			render500(w, "Error counting category posts.")
			return
		}
		rowsPosts, err = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id JOIN post_categories ON posts.id = post_categories.post_id WHERE post_categories.category_id = ? ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, cat, pageSize, offset)
		if err != nil {
			render500(w, "Error loading category posts.")
			return
		}
	} else {
		countQuery = "SELECT COUNT(*) FROM posts"
		if err := db.QueryRow(countQuery).Scan(&totalPosts); err != nil {
			render500(w, "Error counting all posts.")
			return
		}
		rowsPosts, err = db.Query(`SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username FROM posts JOIN users ON posts.user_id = users.id ORDER BY posts.created_at DESC LIMIT ? OFFSET ?`, pageSize, offset)
		if err != nil {
			render500(w, "Error loading all posts.")
			return
		}
	}
	defer rowsPosts.Close()
	for rowsPosts.Next() {
		var p Post
		if err := rowsPosts.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author); err != nil {
			render500(w, "Error reading posts.")
			return
		}
		// Форматируем дату в "02.01.2006 15:04"
		t, err := time.Parse(time.RFC3339, p.CreatedAt)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", p.CreatedAt)
		}
		if err == nil {
			p.CreatedAt = t.Format("02.01.2006 15:04")
		}
		// Лайки/дизлайки
		if err := db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 1", p.ID).Scan(&p.Likes); err != nil {
			p.Likes = 0 // Default value on error
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 0", p.ID).Scan(&p.Dislikes); err != nil {
			p.Dislikes = 0 // Default value on error
		}
		// Получаем категории поста
		catRows, err := db.Query("SELECT categories.id, categories.name FROM categories JOIN post_categories ON categories.id = post_categories.category_id WHERE post_categories.post_id = ?", p.ID)
		if err != nil {
			render500(w, "Error loading post categories.")
			return
		}
		var cats []Category
		for catRows.Next() {
			var c Category
			if err := catRows.Scan(&c.ID, &c.Name); err != nil {
				catRows.Close()
				render500(w, "Error reading post categories.")
				return
			}
			cats = append(cats, c)
		}
		catRows.Close()
		p.Categories = cats
		// Количество комментариев
		if err := db.QueryRow("SELECT COUNT(*) FROM comments WHERE post_id = ?", p.ID).Scan(&p.CommentCount); err != nil {
			p.CommentCount = 0 // Default value on error
		}
		posts = append(posts, p)
	}

	// Параметры для пагинации
	totalPages := (totalPosts + pageSize - 1) / pageSize

	// Если страница пуста и не первая, редирект на предыдущую страницу
	if len(posts) == 0 && page > 1 {
		q := r.URL.Query()
		q.Set("page", fmt.Sprintf("%d", page-1))
		http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
		return
	}

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
		rows, err := db.Query("SELECT id, name FROM categories")
		if err != nil {
			render500(w, "Error loading categories.")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var c Category
			if err := rows.Scan(&c.ID, &c.Name); err != nil {
				render500(w, "Error reading categories.")
				return
			}
			categories = append(categories, c)
		}
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
		if len([]rune(title)) > 20 {
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
			if _, err := db.Exec("INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)", postID, cid); err != nil {
				render500(w, "Error adding post categories.")
				return
			}
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
	if getCurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		templates.ExecuteTemplate(w, "register.html", nil)
		return
	}
	if r.Method == http.MethodPost {
		email := strings.TrimSpace(r.FormValue("email"))
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		// Проверка на пустые поля
		if email == "" || username == "" || password == "" {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "All fields are required."})
			return
		}
		// Email: длина 5-50, валидный формат
		if len([]rune(email)) < 5 || len([]rune(email)) > 50 || !isValidEmail(email) {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Email must be 5-50 chars and valid format."})
			return
		}
		// Username: длина 3-20, только буквы/цифры, Unicode-aware, без пробелов
		if !isValidUsername(username) {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Username: 3-20 letters or digits, no spaces."})
			return
		}
		// Password: длина 6-32
		if len([]rune(password)) < 6 || len([]rune(password)) > 32 {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Password: 6-32 characters."})
			return
		}
		// Проверка уникальности email и username (без учёта регистра)
		var emailExists, usernameExists int
		if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(email) = LOWER(?)", email).Scan(&emailExists); err != nil {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Database error checking email."})
			return
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username) = LOWER(?)", username).Scan(&usernameExists); err != nil {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Database error checking username."})
			return
		}
		if emailExists > 0 {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Email already taken."})
			return
		}
		if usernameExists > 0 {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Username already taken."})
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := db.Exec("INSERT INTO users (email, username, password) VALUES (?, ?, ?)", email, username, string(hash))
		if err != nil {
			templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": "Registration error."})
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// --- Вспомогательные функции для валидации ---
// Email: простая проверка формата
func isValidEmail(email string) bool {
	if strings.Count(email, "@") != 1 {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || len(parts[0]) < 1 || len(parts[1]) < 3 {
		return false
	}
	domain := parts[1]
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || !strings.Contains(domain, ".") {
		return false
	}
	return true
}

// Username: 3-20 символов, только буквы/цифры Unicode, без пробелов
func isValidUsername(username string) bool {
	r := []rune(username)
	if len(r) < 3 || len(r) > 20 {
		return false
	}
	for _, ch := range r {
		if ch == ' ' || ch == '\t' || ch == '\n' {
			return false
		}
		if !(isLetter(ch) || isDigit(ch)) {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= 0x410 && r <= 0x44F) || (r >= 0xC0 && r <= 0xFF) || (r >= 0x0410 && r <= 0x042F) || (r >= 0x0430 && r <= 0x044F) || (r > 127 && r != ' ')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if getCurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodGet {
		templates.ExecuteTemplate(w, "login.html", nil)
		return
	}
	if r.Method == http.MethodPost {
		login := strings.TrimSpace(r.FormValue("login"))
		password := r.FormValue("password")
		var id int
		var username, hash string
		var err error
		// Определяем, email это или username
		if strings.Contains(login, "@") {
			// Email (без учёта регистра)
			err = db.QueryRow("SELECT id, username, password FROM users WHERE LOWER(email) = LOWER(?)", login).Scan(&id, &username, &hash)
		} else {
			// Username (без учёта регистра)
			err = db.QueryRow("SELECT id, username, password FROM users WHERE LOWER(username) = LOWER(?)", login).Scan(&id, &username, &hash)
		}
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
		if _, err := db.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
			templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Database error cleaning sessions"})
			return
		}
		// Добавляем новую сессию
		if _, err := db.Exec("INSERT INTO sessions (user_id, uuid, expires) VALUES (?, ?, ?)", id, sid, expires); err != nil {
			templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Database error creating session"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:    "session",
			Value:   sid,
			Expires: expires,
			Path:    "/",
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// конец loginHandler

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		// Игнорируем ошибку удаления сессии, так как logout должен всегда работать
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
