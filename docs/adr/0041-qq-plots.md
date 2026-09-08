# 0041 — QQ plots rank the sample before it is drawn

Status: accepted; implemented in v1.5.0.

`geom.QQ` compares the column named by X with the standard normal distribution.
The display axes are theoretical quantiles on X and observed values on Y;
the sample is neither standardised nor fitted. A reference line would assert
parameters, so callers add one explicitly when they know them.

Plotting positions are `(i + 0.5) / n`. Every finite observation gets one point,
including ties, and endpoints are excluded. Missing observations are omitted
before ranking. A log display can hide an observation but cannot change the
remaining ranks. The Error missing policy still rejects unplottable input.
Categorical axes are refused because neither of these axes is a category.

The calculation runs in Train, and both axes train on its output. The ECDF's
retained sorting/group buffers serve both marks; its staircase drawing does
not. Build uses pooled scratch, checks plottability once, and projects the
surviving pairs through the coord. Grouping, faceting, legend styles and the
`qq` JSON mark use the existing extension points. Quantile marks do not claim
source-row identity. This is a summary, including where equal values tie.

`stat.QQ` and `stat.AppendQQ` accept a theoretical quantile function;
nil selects `stat.NormalQuantile`. They take sorted input and do not mutate it.
Custom functions stay in Go; to serialize another distribution, materialize
its pairs as data and use Scatter. No function serialization or distribution
registry is introduced, and the core gains no dependency.
