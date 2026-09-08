# 0050 — A ternary chart is a barycentric coord, and its third grid family is not furniture

**Status:** Proposed · **Date:** 2026-09-08 · **Implemented:** —

## Context

A **ternary plot** reads three components that sum to a constant — a rock's
three oxides, a soil's sand, silt and clay, an alloy's three metals, a mixture's
three phases, a classifier's three-class probability — as one point inside an
equilateral triangle. Two of the three are free; the third is what is left.

It is the second-most-common coordinate system in the physical sciences after
polar, it has been standard in petrology, metallurgy, soil science and physical
chemistry for over a century, and the tooling for it is thin everywhere:
Plotly has it, R needs `ggtern`, Python needs `python-ternary` or `mpltern`,
D3 has nothing, Vega-Lite has nothing. **Go has nothing at all.**

`docs/chart-types.md` sorts by machinery. A ternary chart needs exactly one
piece — a coord — and everything else it wants shipped between v0.1 and v0.9:
a scatter, a line, a rectangle for a field, colour, size, a legend, faceting.

## Decision

**`coord.Ternary` reads a panel's two axes as two components of a composition
and derives the third.** X is the first component, Y the second, and the third
is `Sum − x − y`.

```go
p := refract.New(refract.Coord(coord.Ternary()))
p.X(scale.Linear(scale.Domain(0, 1)))
p.Y(scale.Linear(scale.Domain(0, 1)))
p.Add(geom.Scatter(rocks,
	geom.X("quartz"), geom.Y("feldspar"),
	geom.ColorBy("unit", scale.Qualitative())))
```

`coord.TernarySum(100)` is the percentage spelling, which is what most tables
in these fields come in.

### Deriving the third component rather than reading it

The alternative was three named columns. It was rejected because a coord
transforms **a mapped pair** — that is what ADR 0018 built the stage on, and
what makes `scale.Scale` unchanged — and because reading three columns
introduces a consistency question the derivation does not have: a row whose
three columns sum to 0.98 has to be normalised, refused or ignored, and every
answer is wrong for somebody. Deriving the third makes the constraint hold by
construction. A caller whose table genuinely carries three columns divides by
their sum, which is arithmetic at the call site or a column in the source.

### It is the cheapest coord in the package

The barycentric map is **affine**. With a and b on the axes and c = k − a − b,
and the triangle's vertices at A, B and C:

```
P = (a·A + b·B + c·C) / k
```

which is a 2×2 matrix and a translation. Consequences, each of which is a
method that costs a line:

- `Straight()` is **true**. An affine map takes a straight segment to a
  straight segment, so an edge is a `LineTo` and every geom draws exactly what
  it drew under Cartesian. `Polar` and `Smith` both had to build cubics; this
  one does not.
- `Area` is four transformed corners. A `geom.Rect` over a ternary is a
  parallelogram, correctly, and a ternary heatmap over binned compositions
  therefore costs no new mark.
- `Invert` is the inverse matrix, so a tooltip reports the composition and
  `interact` is unchanged.
- `Clip` is the triangle rather than the rectangle, which is the one thing it
  does not inherit.

`Decimates()` is **false**, and unlike `Polar` the argument is a derivation
rather than an observation. Working the map out for the usual orientation gives
`px = ½ + ½b − ½a`: a column of screen is a band of constant *b − a*, not of
constant a. A reduction defined over pixel columns therefore does not measure
what it was defined to measure — which is exactly what `Decimates` asks —
even though nothing about the map is curved.

### The third grid family, and why this record does not widen `Furniture`

A ternary chart has **three** labelled ladders and a panel has **two** tick
lists. This is the constraint [ADR 0033](0033-smith-charts.md) named for the
VSWR circles, arriving a second time, and it is the reason this record exists
rather than being a paragraph in a changelog.

Three things are true and they resolve it between them.

**One.** The constant-a and constant-b families are the images of the two
axes' own ticks, exactly as a Smith chart's two families are. They are grid
lines, they are labelled from `t.Label`, and `render` is untouched.

**Two.** The constant-c family needs no new field to be *drawn*. A `Shape`
carries either a straight run or an `ir.Path`, and a path may hold more than
one subpath. The constant-c line at level v is the diagonal a + b = k − v, and
in the sequence every ternary chart is printed with — the same ladder on all
three edges — it pairs with the X tick at v. So the coord emits it as a second
subpath inside `GridX[i]`, it takes the theme's grid ink like its neighbours,
and `Furniture` gains nothing.

**Three.** What is genuinely missing is that family's **labels**, because
`Furniture` carries one `Label` per tick per side and there is no third side.
That is the seam, and this record declines to spend it, for the reason 0033
gave and [ADR 0049](0049-locus-annotations.md) sharpened: it should be spent
once, on the general problem — a third labelled family here, a graticule's
labels for a projection — and not twice on two thirds of it.

Until it is spent, a ternary chart's third edge is labelled the way ternary
charts are usually labelled anyway: `geom.Note` at the corner, naming the
component. The numeric ladder on that edge is redundant with the other two —
all three read the same sequence in the same direction — which is why the
omission is survivable and why an unlabelled third family is not a broken
chart.

**The alternative considered and not taken** is to draw the third family as a
`geom.Locus` (ADR 0049): it is a family of curves in data space defined by a
formula, the affine coord draws it correctly for free, and it would need
nothing from this record at all. It was not made the default because a locus
takes annotation ink and this is a grid line, so the chart would come out with
two of its three families one colour and the third another. It remains the
right escape hatch for a caller who wants a fourth family, an unusual level
sequence, or a differently-styled ladder, and it costs nothing to leave open.

### The domains are pinned

Both axes run `[0, Sum]` and are not trained, for the reason a Smith chart's
are not: the chart's extent is the whole triangle whatever the data does, and
an axis that autoscaled to a tight cluster of compositions would draw a
triangle that is not the simplex. As on a Smith chart, a zoom therefore
relabels and moves nothing.

## Consequences

| | |
|---|---|
| `render`, `ir`, `geom`, `layout`, `scale` | unchanged |
| `coord` | one type, two or three options, a `Desc` field; the affine case may be worth factoring out, since Cartesian is the identity member of it |
| `spec` | a `"ternary"` coord type, a `sum` field, an orientation field |
| `interact` | unchanged; a hover inverts through the coord and reports a composition |
| `a11y` | unchanged; it never mentions a coord |
| layout | inherited from Polar's situation — a triangle inscribed in the panel leaves three regions of slack, and the corner labels live in them. `coord.TernaryRadius`-shaped mitigation, if it is needed at all |
| Charts unlocked | ternary scatter, ternary line and path, ternary density via `Rect` over binned compositions, QFL and QAP diagrams, the soil texture triangle, three-phase diagrams, flammability diagrams, and the probability simplex |

### And one chart that follows from it

A **Piper diagram** — the standard instrument of water chemistry, two ternaries
projecting into a diamond — is two ternary panels and one Cartesian panel in a
`Grid`, plus the projection arithmetic, which is a pure function of six numbers.
It is a recipe once this exists, and it is worth naming because it is the
second form in this record that no general-purpose library draws.

## Not in scope

- **A sub-triangle ("ternary zoom").** Real in geochemistry, and not a matter
  of moving two domains: three ranges have to stay mutually consistent or the
  region stops being a triangle. It is its own decision.
- **Three named columns**, per the argument above.
- **Normalising a composition.** If it ever becomes a stat it is
  numbers-in-numbers-out and belongs in `stat` beside the others; today it is
  a division.
- **A quaternary (tetrahedral) diagram.** Three free components is three
  dimensions, which is the 3D question and not this one.
- **Labelling the third family**, per the argument above.

## Revisit if

- The third family's labels are asked for by name. That is the moment to spend
  the `Furniture` seam, and it should be spent together with a projection's
  graticule and 0033's Γ-as-input, exactly as 0033 said.
- A ternary chart turns out to be a big-data chart — geochemical surveys are
  large — which would reopen `Decimates`. The honest fix there is a reduction
  defined over the coord's own axis rather than over pixel columns, which is a
  change to the reduction and not to the answer, as 0033 noted for its own.
