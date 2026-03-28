Display / frontend                                                                                                                                 
  - SolidJS/React UI — paste a YouTube URL, get summary + takeaways rendered instantly
  - Browser extension — right-click any YouTube video, get a summary overlay                                                                         
                  
  Pipeline / automation                                                                                                                              
  - Save to a database (Postgres/SQLite) and build a searchable knowledge base of watched videos
  - Feed into a second LLM pass — e.g. cluster takeaways across multiple videos on the same topic                                                    
  - Export to Obsidian/Notion/Logseq as structured notes                                         
                                                                                                                                                     
  API / integrations                                                                                                                                 
  - Webhook output — post results to Slack/Discord when a new video is processed                                                                     
  - RSS/YouTube channel monitor — poll a channel, auto-summarize new uploads                                                                         
  - CLI tool — pipe curl output into jq for quick terminal summaries        
                                                                                                                                                     
  Data / research                                                                                                                                    
  - Batch-process a playlist and diff summaries to track how a topic evolves over time                                                               
  - Extract named entities from takeaways for a topic graph  