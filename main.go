package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

type CaptchaResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type CaptchaData struct {
	ImageID     string `json:"image_id"`
	MainImage   string `json:"main_image"`
	CutoutImage string `json:"cutout_image"`
}

func getRandomCaptcha(db *sql.DB) (*CaptchaData, error) {
	var imageID string
	err := db.QueryRow("SELECT image_id FROM captchas ORDER BY RANDOM() LIMIT 1").Scan(&imageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get random captcha: %v", err)
	}

	return &CaptchaData{
		ImageID:     imageID,
		MainImage:   fmt.Sprintf("/assets/prod/%s/white_fill.png", imageID),
		CutoutImage: fmt.Sprintf("/assets/prod/%s/shape_extract.png", imageID),
	}, nil
}

func main() {
	db, err := sql.Open("sqlite3", "captcha.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	schemaSQL, err := os.ReadFile("schema.sql")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read schema: %v\n", err)
		os.Exit(1)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize schema: %v\n", err)
		os.Exit(1)
	}

	fs := http.FileServer(http.Dir("assets"))
	http.Handle("/assets/", http.StripPrefix("/assets/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		captcha, err := getRandomCaptcha(db)
		if err != nil {
			http.Error(w, "Failed to get captcha", http.StatusInternalServerError)
			return
		}

		tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Captcha Challenge</title>
    <style>
        .container {
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            gap: 20px;
        }
        .captcha-area {
            position: relative;
            border: 2px solid #ccc;
            margin: 10px;
        }
        #mainImage {
            max-width: 100%;
            display: block;
        }
        #cutout {
            cursor: move;
            position: absolute;
            z-index: 1000;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="captcha-area">
            <img id="mainImage" src="{{.MainImage}}" alt="Captcha">
            <img id="cutout" src="{{.CutoutImage}}" alt="Cutout" draggable="true">
        </div>
    </div>

    <script>
        let cutout = document.getElementById('cutout');
        let isDragging = false;
        let currentX;
        let currentY;
        let initialX;
        let initialY;
        let xOffset = 0;
        let yOffset = 0;

        cutout.addEventListener('mousedown', dragStart);
        document.addEventListener('mousemove', drag);
        document.addEventListener('mouseup', dragEnd);

        function dragStart(e) {
            initialX = e.clientX - xOffset;
            initialY = e.clientY - yOffset;

            if (e.target === cutout) {
                isDragging = true;
            }
        }

        function drag(e) {
            if (isDragging) {
                e.preventDefault();
                currentX = e.clientX - initialX;
                currentY = e.clientY - initialY;

                xOffset = currentX;
                yOffset = currentY;

                setTranslate(currentX, currentY, cutout);
            }
        }

        function setTranslate(xPos, yPos, el) {
            el.style.transform = "translate3d(" + xPos + "px, " + yPos + "px, 0)";
        }

        function dragEnd(e) {
            if (isDragging) {
                isDragging = false;
                
                // Get position relative to main image
                let mainImage = document.getElementById('mainImage');
                let mainRect = mainImage.getBoundingClientRect();
                let cutoutRect = cutout.getBoundingClientRect();
                
                let relativeX = Math.round(cutoutRect.left - mainRect.left);
                let relativeY = Math.round(cutoutRect.top - mainRect.top);

                // Validate position
                fetch('/validate', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        imageId: '{{.ImageID}}',
                        x: relativeX,
                        y: relativeY
                    })
                })
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        alert('Correct!');
                        window.location.reload();
                    } else {
                        alert('Try again');
                    }
                });
            }
        }
    </script>
</body>
</html>
`
		t := template.Must(template.New("captcha").Parse(tmpl))
		t.Execute(w, captcha)
	})

	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var request struct {
			ImageID string `json:"imageId"`
			X       int    `json:"x"`
			Y       int    `json:"y"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var validationData ValidationData
		err := db.QueryRow(`
			SELECT image_id, valid_x, valid_y, tolerance, shape_width, shape_height 
			FROM captchas 
			WHERE image_id = ?
		`, request.ImageID).Scan(
			&validationData.ImageID,
			&validationData.ValidX,
			&validationData.ValidY,
			&validationData.Tolerance,
			&validationData.ShapeWidth,
			&validationData.ShapeHeight,
		)
		if err != nil {
			http.Error(w, "Failed to load validation data", http.StatusInternalServerError)
			return
		}

		success := ValidateCaptchaAttempt(validationData, request.X, request.Y)

		response := CaptchaResponse{
			Success: success,
			Message: map[bool]string{true: "Correct!", false: "Try again"}[success],
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	fmt.Println("Server starting on :8080...")
	http.ListenAndServe(":8080", nil)
}
