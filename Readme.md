gomemqueue
==========

Simple In memory queue written in go

Supported interfaces 

'''
1. PUT /memqueue/<queue_name>
   Creates a new queue with name <queue_name>

2. POST /memqueue/<queue_name> 
   Post a message to <queue_name>

3. GET /memqueue/<queue_name> 
   Gets some messages from the queue name named <queue_name>

4. GET /memstreamqueue/<queue_name>
   Stream entries from the queue. 
'''

If multiple streams are connected to the server then the copy of the data will be served to 
each of the connected streams. 

**Warning** Do not use interfaces 3 & 4 simulataneously because stream readers will get priority
and no data will be written to non-streamed reader

