package main

import (
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ernyoke/imger/blur"
	"github.com/ernyoke/imger/edgedetection"
	"github.com/ernyoke/imger/grayscale"
	"github.com/ernyoke/imger/padding"
	_ "github.com/mattn/go-sqlite3"
)

type ValidationData struct {
	ImageID     string `json:"image_id"`     // Original image identifier
	ValidX      int    `json:"valid_x"`      // X coordinate where shape should be placed
	ValidY      int    `json:"valid_y"`      // Y coordinate where shape should be placed
	Tolerance   int    `json:"tolerance"`    // Allowed pixel deviation from exact position
	ShapeWidth  int    `json:"shape_width"`  // Width of the shape
	ShapeHeight int    `json:"shape_height"` // Height of the shape
}

func SaveValidationData(data ValidationData, db *sql.DB) error {
	query := `
		INSERT INTO captchas (image_id, valid_x, valid_y, tolerance, shape_width, shape_height)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, data.ImageID, data.ValidX, data.ValidY, data.Tolerance, data.ShapeWidth, data.ShapeHeight)
	return err
}

func drawLine(img *image.RGBA, p1, p2 image.Point, col color.Color) {
	dx := math.Abs(float64(p2.X - p1.X))
	dy := math.Abs(float64(p2.Y - p1.Y))

	x1, y1 := p1.X, p1.Y
	x2, y2 := p2.X, p2.Y

	sx := 1
	if x1 > x2 {
		sx = -1
	}

	sy := 1
	if y1 > y2 {
		sy = -1
	}

	err := dx - dy
	for {
		if image.Pt(x1, y1).In(img.Bounds()) {
			img.Set(x1, y1, col)
		}
		if x1 == x2 && y1 == y2 {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func superformula(phi float64, a, b float64, m float64, n1, n2, n3 float64) float64 {
	term1 := math.Pow(math.Abs(math.Cos(m*phi/4)/a), n2)
	term2 := math.Pow(math.Abs(math.Sin(m*phi/4)/b), n3)
	return math.Pow(term1+term2, -1/n1)
}

func shape_gen_in_region(x, y, w, h int) []image.Point {
	rand.Seed(time.Now().UnixNano())

	numPoints := 361
	points := make([]image.Point, 0, numPoints)

	a := rand.Float64()*0.5 + 0.6
	b := rand.Float64()*0.5 + 0.6
	m := float64(rand.Intn(10) + 3)
	n1 := rand.Float64()*3 + 1
	n2 := rand.Float64()*3 + 1
	n3 := rand.Float64()*3 + 1

	scale := float64(min(w, h)) * 0.4
	cx := float64(x + w/2)
	cy := float64(y + h/2)

	for i := range numPoints {
		phi := float64(i) * math.Pi / 180
		r := superformula(phi, a, b, m, n1, n2, n3)

		xp := cx + scale*r*math.Cos(phi)
		yp := cy + scale*r*math.Sin(phi)

		points = append(points, image.Point{X: int(xp), Y: int(yp)})
	}

	return points
}

func SplitAny(s string, seps string) []string {
	splitter := func(r rune) bool {
		return strings.ContainsRune(seps, r)
	}
	return strings.FieldsFunc(s, splitter)
}

func findMaxHeat(heatmap [][]int) (maxVal, maxI, maxJ int) {
	maxVal = heatmap[0][0]
	maxI, maxJ = 0, 0

	for i := range len(heatmap) {
		for j := range len(heatmap[i]) {
			if heatmap[i][j] > maxVal {
				maxVal = heatmap[i][j]
				maxI, maxJ = i, j
			}
		}
	}

	return
}

func replaceShapeContentWithWhite(img image.Image, shapePoints []image.Point, outPath string) error {
	if len(shapePoints) < 3 {
		return fmt.Errorf("not enough points to create a shape: %d", len(shapePoints))
	}

	bounds := img.Bounds()
	mask := image.NewAlpha(bounds)

	poly := &image.Alpha{
		Pix:    make([]uint8, bounds.Dx()*bounds.Dy()), // Create pixel buffer
		Stride: bounds.Dx(),                            // Width of one row in bytes
		Rect:   bounds,                                 // Image dimensions
	}

	fillPolygon(poly, shapePoints)
	draw.Draw(mask, bounds, poly, bounds.Min, draw.Src)

	output := image.NewRGBA(bounds)
	draw.Draw(output, bounds, img, bounds.Min, draw.Src)

	white := color.RGBA{255, 255, 255, 255} // Fully opaque white

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if mask.AlphaAt(x, y).A > 0 {
				output.Set(x, y, white)
			}
		}
	}

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	return png.Encode(outFile, output)
}

func extractShapeContent(img image.Image, shapePoints []image.Point, outPath string) error {
	if len(shapePoints) < 3 {
		return fmt.Errorf("not enough points to create a shape: %d", len(shapePoints))
	}
	bounds := img.Bounds()
	mask := image.NewAlpha(bounds)

	poly := &image.Alpha{
		Pix:    make([]uint8, bounds.Dx()*bounds.Dy()),
		Stride: bounds.Dx(),
		Rect:   bounds,
	}

	fillPolygon(poly, shapePoints)
	draw.Draw(mask, bounds, poly, bounds.Min, draw.Src)
	output := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if mask.AlphaAt(x, y).A > 0 {
				output.Set(x, y, img.At(x, y))
			} else {
				output.Set(x, y, color.Transparent)
			}
		}
	}

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	return png.Encode(outFile, output)
}

func fillPolygon(img *image.Alpha, points []image.Point) {

	bounds := img.Bounds()
	minY := points[0].Y
	maxY := points[0].Y
	for _, p := range points {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	if minY < bounds.Min.Y {
		minY = bounds.Min.Y
	}

	if maxY >= bounds.Max.Y {
		maxY = bounds.Max.Y - 1
	}

	for y := minY; y <= maxY; y++ {
		var intersections []int
		for i := range points {
			j := (i + 1) % len(points)
			p1 := points[i]
			p2 := points[j]

			if (p1.Y <= y && p2.Y > y) || (p2.Y <= y && p1.Y > y) {
				if p1.Y == p2.Y {
					intersections = append(intersections, p1.X)
					intersections = append(intersections, p2.X)
				} else {
					x := p1.X + (y-p1.Y)*(p2.X-p1.X)/(p2.Y-p1.Y)
					intersections = append(intersections, x)
				}
			}
		}

		sort.Ints(intersections)

		for i := 0; i < len(intersections); i += 2 {
			if i+1 < len(intersections) {
				startX := intersections[i]
				endX := intersections[i+1]

				if startX < bounds.Min.X {
					startX = bounds.Min.X
				}
				if endX >= bounds.Max.X {
					endX = bounds.Max.X - 1
				}

				for x := startX; x <= endX; x++ {
					img.SetAlpha(x, y, color.Alpha{A: 255})
				}
			}
		}
	}
}

func findShapeCorners(points []image.Point) (topLeft, topRight, bottomLeft, bottomRight image.Point) {
	if len(points) == 0 {
		return
	}

	topLeft = points[0]
	topRight = points[0]
	bottomLeft = points[0]
	bottomRight = points[0]

	for _, p := range points {
		if p.X <= topLeft.X && p.Y <= topLeft.Y {
			topLeft = p
		}
		if p.X >= topRight.X && p.Y <= topRight.Y {
			topRight = p
		}
		if p.X <= bottomLeft.X && p.Y >= bottomLeft.Y {
			bottomLeft = p
		}
		if p.X >= bottomRight.X && p.Y >= bottomRight.Y {
			bottomRight = p
		}
	}

	return
}

func generateCaptcha() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("please provide an image file path")
	}

	// Get the working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}
	// Project root is two directories up from cmd/scripts
	projectRoot := filepath.Join(workDir, "..", "..")

	filePath := os.Args[1]
	fileName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	outBasePath := filepath.Join(projectRoot, "assets", "prod", fileName)

	// Create output directory
	if err := os.MkdirAll(outBasePath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite3", filepath.Join(projectRoot, "captcha.db"))
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize schema if not exists
	schemaSQL, err := os.ReadFile(filepath.Join(projectRoot, "schema.sql"))
	if err != nil {
		return fmt.Errorf("failed to read schema file: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		return fmt.Errorf("failed to initialize schema: %v", err)
	}

	fmt.Println("[INFO] Reading file from", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %v", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	grey := grayscale.Grayscale(img)
	blur_img, err := blur.GaussianBlurGray(grey, 7, 3, padding.BorderConstant)
	if err != nil {
		return fmt.Errorf("failed to apply blur: %v", err)
	}

	edge_img, err := edgedetection.CannyGray(blur_img, 10, 100, 3)
	if err != nil {
		return fmt.Errorf("failed to detect edges: %v", err)
	}

	// Calculate Heatmap
	blockRows := 5
	blockCols := 5
	blockW := w / blockCols
	blockH := h / blockRows

	heatmap := make([][]int, blockRows)
	for i := range heatmap {
		heatmap[i] = make([]int, blockCols)
	}

	for y := range h {
		for x := range w {
			gray := edge_img.GrayAt(x, y).Y
			if gray > 128 {
				row := y / blockH
				col := x / blockW
				if row < blockRows && col < blockCols {
					heatmap[row][col]++
				}
			}
		}
	}

	max, row, col := findMaxHeat(heatmap)
	x := col * blockW
	y := row * blockH

	fmt.Printf("[INFO] Max heat at x=%d y=%d heat=%d\n", x, y, max)

	// Generate random shape
	shapePoints := shape_gen_in_region(x, y, blockW, blockH)
	fmt.Printf("[INFO] Generated shape with %d points\n", len(shapePoints))
	tl, tr, bl, br := findShapeCorners(shapePoints)
	fmt.Printf("[INFO] Shape corners: tl=%v tr=%v bl=%v br=%v\n", tl, tr, bl, br)

	// Calculate shape dimensions
	shapeWidth := tr.X - tl.X
	shapeHeight := bl.Y - tl.Y

	// Create validation data
	validationData := ValidationData{
		ImageID:     fileName,
		ValidX:      x,
		ValidY:      y,
		Tolerance:   10,
		ShapeWidth:  shapeWidth,
		ShapeHeight: shapeHeight,
	}

	// Save validation data to SQLite
	if err := SaveValidationData(validationData, db); err != nil {
		return fmt.Errorf("failed to save validation data: %v", err)
	}
	fmt.Println("[INFO] Validation data saved to database")

	// Save edge detection result
	edgeFileName := filepath.Join(outBasePath, "edge.png")
	edgeFileOut, err := os.Create(edgeFileName)
	if err != nil {
		return fmt.Errorf("failed to create edge file: %v", err)
	}
	if err := png.Encode(edgeFileOut, edge_img); err != nil {
		edgeFileOut.Close()
		return fmt.Errorf("failed to encode edge image: %v", err)
	}
	edgeFileOut.Close()
	fmt.Println("[INFO] Edge file saved at", edgeFileName)

	// Save shape extract
	outPath := filepath.Join(outBasePath, "shape_extract.png")
	if err := extractShapeContent(img, shapePoints, outPath); err != nil {
		return fmt.Errorf("failed to extract shape: %v", err)
	}
	fmt.Println("[INFO] Shape content extracted and saved to", outPath)

	// Save white-filled version
	whitePath := filepath.Join(outBasePath, "white_fill.png")
	if err := replaceShapeContentWithWhite(img, shapePoints, whitePath); err != nil {
		return fmt.Errorf("failed to create white-filled shape: %v", err)
	}
	fmt.Println("[INFO] White-filled shape created and saved to", whitePath)

	// Save debug image with shape outline
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, img, bounds.Min, draw.Src)
	red := color.RGBA{255, 0, 0, 255}

	for i := range len(shapePoints) - 1 {
		drawLine(dst, shapePoints[i], shapePoints[i+1], red)
	}
	drawLine(dst, shapePoints[0], shapePoints[len(shapePoints)-1], red)

	outFileName := filepath.Join(outBasePath, "debug.png")
	outFile, err := os.Create(outFileName)
	if err != nil {
		return fmt.Errorf("failed to create debug file: %v", err)
	}
	if err := png.Encode(outFile, dst); err != nil {
		outFile.Close()
		return fmt.Errorf("failed to encode debug image: %v", err)
	}
	outFile.Close()

	fmt.Println("[INFO] Debug image saved at", outFileName)
	return nil
}

func main() {
	if err := generateCaptcha(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
