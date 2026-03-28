# Architecture decision record

1. For high ram usage (70%) is recommend to use 4k context to avoid OOM.
```
  1. Refine / Rolling Window — best fit for transcripts

  Process the transcript in sequential chunks, carrying a running summary forward:

  chunk1 → summary1
  summary1 + chunk2 → summary2
  summary2 + chunk3 → summary3  (final)
  - Preserves narrative flow and order
  - Sequential (can't parallelize), but transcripts are linear so that's fine
  - ~2000 token chunks, ~200 token overlap at boundaries

  2. Map-Reduce — best for takeaways

  Summarize each chunk independently, then merge the results:
  chunk1 → key_points_1
  chunk2 → key_points_2  (parallel)
  chunk3 → key_points_3
  [key_points_1,2,3] → final takeaways
  - Parallelizable = faster
  - Great for extracting facts/takeaways since they don't need continuity
  - Not ideal for narrative summary
```

2. When summarize long text we will get "lost in the middle context" when we fit all chunks into one context window at once. So i was thinking to use:
```
compounding compression of early content:                                                     
  
  chunk1 → summary_1                                                                                                                   
  summary_1 + chunk2 → summary_2   (chunk1 content now compressed once)                                                              
  summary_2 + chunk3 → summary_3   (chunk1 content now compressed twice)
```

2. SSE progress events from LLM to client for better UX instead of circular loading (while waiting all the summarize result from LLM done)