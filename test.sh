#!/bin/bash

./gomemqueue &

#create a new queue
curl http://localhost:9191/memqueue/nq -X PUT

#post some messages 
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro1'
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro2'
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro3'
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro4'

#get all those messages 
echo "Should get 4 messages"
curl http://localhost:9191/memqueue/nq

#post some messages 
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro1'
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro2'
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro3'
curl http://localhost:9191/memqueue/nq -X POST -d 'hey bro4'

# stream all the messages back
echo "reading 4 messages"
curl http://localhost:9191/memqueuestream/nq &

#delete the queue
curl http://localhost:9191/memqueue/nq -X DELETE
