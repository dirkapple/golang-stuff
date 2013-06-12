#include <string.h>
#include "helper.h"

//internal callback functions
int xml_write_callback(void *ctx, char *buffer, int len) {
	if (len > 0) {
		xmlNodeWriteCallback(ctx, buffer, len);
	}
  	return len;
}

int close_callback(void * ctx) {
  	return 0;
}

xmlDoc* newEmptyXmlDoc() {
	//why does xmlNewDoc NOT call xmlInitParser like other parse functions?
	xmlInitParser();
	return xmlNewDoc(BAD_CAST XML_DEFAULT_VERSION); 
}

xmlElementType getNodeType(xmlNode *node) { return node->type; }

void xmlFreeChars(char *buffer) { 
	if (buffer) {
		xmlFree((xmlChar*)buffer); 
	}
}

char *xmlDocDumpToString(xmlDoc *doc, void *encoding, int format) {
	xmlChar *buff;
	int buffersize;
	xmlDocDumpFormatMemoryEnc(doc, &buff, &buffersize, (char*)encoding, format);
	return (char*)buff;
}

char *htmlDocDumpToString(htmlDocPtr doc, int format) {
	xmlChar *buff;
	int buffersize;
	htmlDocDumpMemoryFormat(doc, &buff, &buffersize, format);
	return (char*)buff;
}

xmlDoc* xmlParse(void *buffer, int buffer_len, void *url, void *encoding, int options, void *error_buffer, int error_buffer_len) {
	const char *c_buffer       = (char*)buffer;
	const char *c_url          = (char*)url;
	const char *c_encoding     = (char*)encoding;
	xmlDoc *doc = NULL;
	
	xmlResetLastError();
	doc = xmlReadMemory(c_buffer, buffer_len, c_url, c_encoding, options);

	if(doc == NULL) {
		xmlErrorPtr error;
	    xmlFreeDoc(doc);
	    error = xmlGetLastError();
		if(error != NULL && error_buffer != NULL && error->level >= XML_ERR_ERROR) {
			char *c_error_buffer = (char*)error_buffer;
			if (error->message != NULL) {
				strncpy(c_error_buffer, error->message, error_buffer_len-1);
				c_error_buffer[error_buffer_len-1] = '\0';
			}
			else {
				//snprintf(c_error_buffer, error_buffer_len, "xml parsing error:%d", error->code);
			}
		}
	}
	return doc;
}

xmlNode* xmlParseFragment(void *doc, void *buffer, int buffer_len, void *url, int options, void *error_buffer, int error_buffer_len) {
	xmlNodePtr root_element = NULL;
	xmlParserErrors errCode;
	errCode = xmlParseInNodeContext((xmlNodePtr)doc, buffer, buffer_len, options, &root_element);
	if (errCode != XML_ERR_OK) {
		if (error_buffer != NULL && error_buffer_len > 0) {
			//char *c_error_buffer = (char*)error_buffer;
			//snprintf(c_error_buffer, error_buffer_len, "xml fragemnt parsing error (xmlParserErrors):%d", errCode);
		}
		printf("errorcode %d\n", errCode);
		return NULL;
	} 
	return root_element;
}

xmlNode* xmlParseFragmentAsDoc(void *doc, void *buffer, int buffer_len, void *url, void *encoding, int options, void *error_buffer, int error_buffer_len) {
	xmlDoc* tmpDoc = NULL;
	xmlNode* tmpRoot = NULL;
	tmpDoc = xmlReadMemory((char*)buffer, buffer_len, (char*)url, (char*)encoding, options);
	if (tmpDoc == NULL) {
		return NULL;
	}
	tmpRoot = xmlDocGetRootElement(tmpDoc);
	if (tmpRoot == NULL) {
		return NULL;
	}
	tmpRoot = xmlDocCopyNode(tmpRoot, doc, 1);
	xmlFreeDoc(tmpDoc);
	return tmpRoot;
}

void xmlSetContent(void *gonode, void *n, void *content) {
	xmlNode *node = (xmlNode*)n;
	xmlNode *child = node->children;
	xmlNode *next = NULL;
	char *encoded = xmlEncodeSpecialChars(node->doc, content);
	if (encoded) {
		while (child) {
			next = child->next ;
			xmlUnlinkNode(child);
			//xmlFreeNode(child);
			xmlUnlinkNodeCallback(child, gonode);
			child = next ;
	  	}
	  	xmlNodeSetContent(node, (xmlChar*)encoded);
		xmlFree(encoded);
	}
}

int xmlUnlinkNodeWithCheck(xmlNode *node) {
	if (xmlNodePtrCheck(node->parent)) {
		xmlUnlinkNode(node);
		return 1;
	}
	return 0;
}

int xmlNodePtrCheck(void *node) {
	if (node == (void*)(-1)) 
		return 0;
	return 1;
}

int xmlSaveNode(void *wbuffer, void *node, void *encoding, int options) {
	xmlSaveCtxtPtr savectx;
	const char *c_encoding = (char*)encoding;
	
	savectx = xmlSaveToIO(
	      (xmlOutputWriteCallback)xml_write_callback,
	      (xmlOutputCloseCallback)close_callback,
	      wbuffer,
	      encoding,
	      options
	  );
	xmlSaveTree(savectx, (xmlNode*)node);
	return xmlSaveClose(savectx);
}

