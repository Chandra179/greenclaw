package service

func (d *Dependencies) ExtractYoutube() {
	// call youtube pkg to get the transcript first from video if exist if not
	// download audio then extract the transcript using whisper-service @pkg/youtube
	// then store the result to graphdb
}
