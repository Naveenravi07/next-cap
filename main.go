package main

import (
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
	"strings"
	"time"

	"github.com/ernyoke/imger/edgedetection"
	"github.com/ernyoke/imger/grayscale"
)

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

func shape_gen(width, height int) []image.Point {
	rand.Seed(time.Now().UnixNano())

	numPoints := 361
	points := make([]image.Point, 0, numPoints)

	a := rand.Float64()*0.5 + 0.6
	b := rand.Float64()*0.5 + 0.6
	m := float64(rand.Intn(10) + 3)
	n1 := rand.Float64()*3 + 1
	n2 := rand.Float64()*3 + 1
	n3 := rand.Float64()*3 + 1

	scale := float64(min(width, height)) * 0.2
	cx := float64(width) / 2
	cy := float64(height) / 2

	for i := range numPoints {
		phi := float64(i) * math.Pi / 180
		r := superformula(phi, a, b, m, n1, n2, n3)

		x := cx + scale*r*math.Cos(phi)
		y := cy + scale*r*math.Sin(phi)

		points = append(points, image.Point{X: int(x), Y: int(y)})
	}

	return points
}

func SplitAny(s string, seps string) []string {
	splitter := func(r rune) bool {
		return strings.ContainsRune(seps, r)
	}
	return strings.FieldsFunc(s, splitter)
}

func main() {
	filePath := os.Args[1]
	outBasePath := "assets/prod/"

	fmt.Println("[INFO] Reading file from ", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	fileName := SplitAny(file.Name(), "/ .")[2]

	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	//apply gaussian filter here
	grey := grayscale.Grayscale(img)
	edge_img, err := edgedetection.CannyGray(grey, 15, 45, 5)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	edgeFileName := outBasePath + fileName + "_edge.png"
	edgeFileOut, err := os.Create(edgeFileName)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	png.Encode(edgeFileOut, edge_img)
	fmt.Println("[INFO] Edge file saved at ", edgeFileName)

	// Gaussian filter end

	shapePoints := shape_gen(w, h)
	fmt.Println("[INFO] Generated shape with", len(shapePoints), "points.")

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, img, bounds.Min, draw.Src)
	red := color.RGBA{255, 0, 0, 255}

	for i := range len(shapePoints) - 1 {
		drawLine(dst, shapePoints[i], shapePoints[i+1], red)
	}
	drawLine(dst, shapePoints[0], shapePoints[len(shapePoints)-1], red)

	outFileName := outBasePath + fileName + "_out.png"
	outFile, err := os.Create(outFileName)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	defer outFile.Close()
	defer file.Close()

	png.Encode(outFile, dst)
	fmt.Println("Captcha image saved at ", outFileName)
}
