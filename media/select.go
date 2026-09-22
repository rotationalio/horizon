package media

// Returns a list of media types that are allowed as options to be selected in a form.
// This is not the entire list of media types, but rather the subset that are allowed
// to be used in inference via Horizon's API. If new options are required then they
// should be added to the list of allowed media types.
//
// NOTE: This list is cached and rendered as a static HTMX template for the front-end.
func MimeTypeOptions() []Type {
	return []Type{
		/*ApplicationAll,
		 AudioAll,
		FontAll,
		ImageAll, */
		/* TextAll, */
		/* VideoAll, */
		/* ApplicationCalendarJSON,
		ApplicationGeoJSON, */
		ApplicationJSON,
		/* ApplicationPDF,
		ApplicationXML,
		ApplicationYAML, */
		/* 		AudioMP4, */
		ImageHEIC,
		ImageJPEG,
		ImagePNG,
		ImageTIFF,
		/* MultipartFormData, */
		/* TextCalendar,
		TextCSV,
		TextHTML, */
		TextMarkdown,
		TextPlain,
		/* 		VideoH264,
		   		VideoMP4,
		   		VideoMPEG, */
	}
}
