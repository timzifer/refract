# 0042 — A colour ramp may compress its domain or cut it into classes

Status: accepted; implemented in v1.6.0.

`scale.Sequential` and `scale.Diverging` interpolated a ramp linearly across the
domain and offered nothing else. A quantity spanning orders of magnitude —
which is what `stat.Bin` and `geom.Hexbin` produce — had every value but the
largest few round to one end of the ramp, and there was no way to colour it
logarithmically, in classes, or by quantile. The gap was the colour channel's
alone: the positional channel had `Log` and `SymLog` from v0.3.

## The transform is a separate choice from the kind

`ColorLog` and `ColorSymLog` are options rather than constructors, and
`ColorDesc.Transform` is a field beside `ColorDesc.Kind` rather than a value of
it. A diverging ramp over a log-fold change is diverging *and* logarithmic;
folding the two words into one would make one of them unsayable. Vega-Lite
spells a colour transform as the scale's `type`, which refract cannot — `type`
there already carries "sequential", "diverging" and "qualitative" — so a
document writing `"type": "log"` is read as a sequential log ramp, the courtesy
`"nominal"` already gets, and a scale written back out says both words.

A log domain is strictly positive, exactly as `Log`'s is. Training ignores zero
and negative values and `Color` returns the undefined colour for one. Clamping
an empty bin to the low end of the ramp would paint it the colour of the rarest
observation, which is inventing a count; with the default transparent undefined
colour the empty cells of a heatmap are the background, which is what a reader
expects anyway.

On a diverging scale the transform runs on the signed deviation from the
centre, not on the value, so a logarithm there is a symmetric one. A plain
logarithm has nothing to say about a negative deviation, and nothing to say
about zero — which on a diverging scale is the centre itself, the one value
whose colour is not in question.

`pow` and `sqrt` are deliberately absent. They are a dozen lines in the colour
channel and a new positional `Kind` behind it, for the reason the next section
gives.

## The colourbar asks the scale three questions

A bar had the linear reading baked in twice: it built a `scale.Linear` for its
tick values and sampled its gradient at `lo + t*(hi-lo)`. Over five decades that
spends thirty of thirty-two stops on the top decade and labels the bar at round
numbers a log ramp does not put where the labels sit.

`scale.ColorTransformer` is the optional interface that replaces both. It
answers which values are worth labelling (`ColorAxis`), where a value sits on
the bar (`ColorPosition`), and which value the ramp reaches at a point of it
(`ColorValueAt`). Stops are sampled through the third, so they are even along
the *bar* rather than across the domain, and thirty-two are enough again.

Splitting labelling from placement is what makes the interface work for a
diverging scale whose centre is not zero: its axis is linear because a
symmetric logarithm centred on 25 has no round numbers of its own to offer, but
its ticks are placed where the ramp puts them. It is also why `pow` waits — the
axis is a real `Scale` over the domain, and there is no positional `Pow` to hand
it.

`ColorTransform` is on the interface too, so a caller that supplies its own
compression can tell the scale is already doing the job. `geom.Hexbin` takes
its cell fraction linearly when the scale carries a transform, rather than
taking a logarithm of a logarithm.

## Classed scales are a third thing, not a variation

`Threshold`, `Quantize` and `Quantile` take a number in and return one of a
finite set of colours. That is neither `ColorScale`'s continuum nor
`DiscreteColorScale`'s categories: the input is a quantity, so a layer binds it
through `geom.ColorBy` and reads it through `Color`, but the output is a short
list, so a reader can name the class a mark is in rather than estimate a value
from a shade.

They are one implementation because they differ only in where the boundaries
come from — outside the data, from the domain, from the distribution. The
domain, the transform, the undefined colour and which colour a class gets are
shared, and writing them three times is how three scales drift apart. Under
`ColorLog`, `Quantize`'s equal classes are decades, which is the point of
asking for both.

A classed scale still contributes a colourbar rather than legend entries,
because its classes are intervals of a quantity and not names. The bar is drawn
in bands and labelled at the boundaries only: a round number inside a class
would invite interpolation across a step that has no inside.

The bands are as tall as their classes are wide. Equal-height blocks are the
usual choropleth legend and would be easier to label, and they would put the
boundaries somewhere they are not — for a quantile scale that is the one thing
the bar has to show, because classes of very different widths are exactly what
"equally many observations in each" looks like.

`Color` allocates nothing. It is called once per mark, so the class index is
computed against boundaries the scale holds rather than against a list built on
the spot.

## Quantile keeps its sample, and sorts it in Train

Quantiles cannot be computed from a running minimum and maximum, so `Quantile`
is the one colour scale that retains what it is trained on. Both ways to bound
that memory — subsampling into a reservoir, or a t-digest — make the result
depend on the order rows arrived in, and [ADR 0012](0012-parallel-panels.md)
requires a parallel render to be byte-identical to a serial one. Keeping every
value and sorting it is order-independent, so that is what it does, and the
cost is documented on the constructor rather than hidden. A live chart hands
each frame a scale of its own, or computes the boundaries once and pins them
with `Threshold`.

The boundaries are recomputed at the end of `Train` rather than at the first
read. Training is serial and drawing is not: panels are built on separate
goroutines and share the layer's colour scale, so a scale that sorted itself
when a mark first asked for a colour would be sorting itself from several
panels at once. `Train` is called once per layer with a whole column, so this
is a handful of sorts and not one per row.

## Consequences

`ColorGuide.Key` read the ramp at three points, which merged two bars that
differed only between their ends — a transform, or where the classes cut. It
now reads sixteen and appends the breaks.

Everything here is additive. `ColorScale` gained no method: the three new
capabilities are optional interfaces beside it, in the shape
[ADR 0020](0020-discrete-colour-and-multi-entry-legends.md) established for
`DiscreteColorScale`. A scale that implements none is read as a linear ramp
over its domain, which is what every colour scale was before.
