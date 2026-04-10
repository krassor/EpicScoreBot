package report

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
)

// PieSlice represents a single slice of a pie chart.
type PieSlice struct {
	Label string
	Value float64
	Color string
}

// palette for SVG pie chart slices.
var palette = []string{
	"#4e79a7", "#f28e2b", "#e15759", "#76b7b2",
	"#59a14f", "#edc948", "#b07aa1", "#ff9da7",
	"#9c755f", "#bab0ac",
}

const Radius = 100

type Point struct {
	Label string
	Value int
}

type CircleDiagram struct {
	Name   string
	Points []Point
}

// svgPieChart generates an inline SVG pie chart from the given slices.
// width/height are in pixels. Returns raw SVG markup.
func svgPieChart(title string, slices []PieSlice, width, height int) string {
	if len(slices) == 0 {
		return ""
	}

	var total float64
	for _, s := range slices {
		total += s.Value
	}
	if total == 0 {
		return ""
	}

	cx := float64(width) / 2
	cy := float64(height)/2 - 10 // leave room for title
	r := math.Min(cx, cy) - 30   // padding for legend

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&sb, `<text x="%g" y="18" text-anchor="middle" font-size="14" font-weight="bold" fill="#333">%s</text>`, cx, title)

	startAngle := -math.Pi / 2
	for i, s := range slices {
		frac := s.Value / total
		angle := frac * 2 * math.Pi
		endAngle := startAngle + angle

		x1 := cx + r*math.Cos(startAngle)
		y1 := cy + r*math.Sin(startAngle) + 25
		x2 := cx + r*math.Cos(endAngle)
		y2 := cy + r*math.Sin(endAngle) + 25

		largeArc := 0
		if angle > math.Pi {
			largeArc = 1
		}

		color := palette[i%len(palette)]

		if len(slices) == 1 {
			// Full circle — draw a circle instead of a path.
			fmt.Fprintf(&sb, `<circle cx="%g" cy="%g" r="%g" fill="%s"/>`,
				cx, cy+25, r, color)
		} else {
			fmt.Fprintf(&sb, `<path d="M %g %g A %g %g 0 %d 1 %g %g L %g %g Z" fill="%s"/>`,
				x1, y1, r, r, largeArc, x2, y2, cx, cy+25, color)
		}

		// Legend item.
		ly := float64(height) - float64(len(slices)-i)*16
		fmt.Fprintf(&sb, `<rect x="5" y="%g" width="10" height="10" fill="%s"/>`, ly, color)
		fmt.Fprintf(&sb, `<text x="20" y="%g" font-size="11" fill="#333">%s: %.0f (%.0f%%)</text>`,
			ly+10, s.Label, s.Value, frac*100)

		startAngle = endAngle
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// BuildRiskProbabilityDiagram creates a CircleDiagram radar chart for risk
// probability averages. Each axis is "Риск N".
func BuildRiskProbabilityDiagram(risks []RiskReportData) string {
	if len(risks) < 1 {
		return ""
	}
	d := CircleDiagram{Name: "Вероятности рисков"}
	for i, r := range risks {
		avg := avgInts(r.Probabilities)
		d.Points = append(d.Points, Point{
			Label: fmt.Sprintf("Риск %d", i+1),
			Value: int(math.Round(avg * 100)), // scale for better visibility
		})
	}
	return d.Render()
}

// BuildRiskImpactDiagram creates a CircleDiagram radar chart for risk
// impact averages. Each axis is "Риск N".
func BuildRiskImpactDiagram(risks []RiskReportData) string {
	if len(risks) < 1 {
		return ""
	}
	d := CircleDiagram{Name: "Влияние рисков"}
	for i, r := range risks {
		avg := avgInts(r.Impacts)
		d.Points = append(d.Points, Point{
			Label: fmt.Sprintf("Риск %d", i+1),
			Value: int(math.Round(avg * 100)),
		})
	}
	return d.Render()
}

// BuildRiskCoefficientDiagram creates a CircleDiagram radar chart for risk
// coefficients expressed as percentages (e.g. ×1.10 → 10%).
func BuildRiskCoefficientDiagram(risks []RiskReportData) string {
	if len(risks) < 1 {
		return ""
	}
	d := CircleDiagram{Name: "Коэффициенты рисков"}
	for i, r := range risks {
		pct := int(math.Round((r.Coefficient - 1) * 100))
		d.Points = append(d.Points, Point{
			Label: fmt.Sprintf("Риск %d", i+1),
			Value: pct,
		})
	}
	return d.Render()
}

// avgInts returns the arithmetic mean of an int slice.
func avgInts(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}

func (c *CircleDiagram) Render() string {
	res :=
		fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"500\" height=\"500\" viewBox=\"-250 -250 500 500\" role=\"img\" aria-label=%s>\n", c.Name) +
			fmt.Sprintf("<circle r=\"%s\" fill=\"none\" stroke=\"#ddd\" stroke-width=\"0.8\"/>\n", strconv.Itoa(Radius*0.25)) +
			fmt.Sprintf("<circle r=\"%s\" fill=\"none\" stroke=\"#ddd\" stroke-width=\"0.8\"/>\n", strconv.Itoa(Radius*0.5)) +
			fmt.Sprintf("<circle r=\"%s\" fill=\"none\" stroke=\"#ddd\" stroke-width=\"0.8\"/>\n", strconv.Itoa(Radius*0.75)) +
			fmt.Sprintf("<circle r=\"%s\" fill=\"none\" stroke=\"#ddd\" stroke-width=\"0.8\"/>\n", strconv.Itoa(Radius)) +
			c.renderAxis()

	return res
}

func (c *CircleDiagram) renderAxis() string {

	n := len(c.Points)
	if n < 1 {
		return ""
	}

	maxValue := c.maxValue()
	k := float64(Radius) / float64(maxValue)

	phi0 := -math.Pi / 2

	log := slog.With("func", "renderAxis")

	log.Info(
		"render info",
		slog.Int("n", n),
	)

	type xy struct {
		x, y int
	}

	var xyArray []xy
	if n < 3 {
		xyArray = make([]xy, n+1)
		xyArray[n] = xy{x: 0, y: 0}
	} else {
		xyArray = make([]xy, n)
	}

	var res strings.Builder
	res.WriteString("<g stroke=\"#999\" stroke-width=\"1\">\n")

	for i := range n {
		rad := phi0 + ((float64(i*(360/n)) * math.Pi) / 180)

		fmt.Fprintf(&res, "<line x1=\"0\" y1=\"0\" x2=\"%s\" y2=\"%s\"/>\n",
			strconv.Itoa(int(math.Round((Radius * math.Cos(rad))))),
			strconv.Itoa(int(math.Round((Radius * math.Sin(rad))))))

		xyArray[i] = xy{
			x: int(math.Round((k * float64(c.Points[i].Value) * math.Cos(rad)))),
			y: int(math.Round((k * float64(c.Points[i].Value) * math.Sin(rad)))),
		}
	}

	res.WriteString("</g>\n")
	res.WriteString("<g font-family=\"Inter, sans-serif\" font-size=\"12\" fill=\"#333\" text-anchor=\"middle\">\n")

	for i := range n {

		rad := phi0 + ((float64(i*(360/n)) * math.Pi) / 180)

		fmt.Fprintf(&res, "<text x=\"%s\" y=\"%s\">%s</text>\n",
			strconv.Itoa(int(math.Round((1.1 * Radius * math.Cos(rad))))),
			strconv.Itoa(int(math.Round((1.1 * Radius * math.Sin(rad))))),
			c.Points[i].Label)
	}

	res.WriteString("</g>\n")
	res.WriteString("<polygon points=\"\n")

	for i := range n {
		fmt.Fprintf(&res, "%s,%s ",
	strconv.Itoa(xyArray[i].x),
	strconv.Itoa(xyArray[i].y))
	}

	res.WriteString("\" fill=\"#1f77b4\" fill-opacity=\"0.10\" stroke=\"#1f77b4\" stroke-width=\"2\"/>\n")
	res.WriteString("</svg>")
	return res.String()
}

func (c *CircleDiagram) maxValue() int {
	if len(c.Points) == 0 {
		return 0
	}

	max := c.Points[0].Value
	for _, p := range c.Points[1:] {
		if p.Value > max {
			max = p.Value
		}
	}
	return max
}
