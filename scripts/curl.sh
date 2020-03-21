curl -v -H "Content-type: application/json" -X POST -d '{"loginid":"small@126.com","password":"1qaz@wsx"}' http://localhost:5001/login
curl -v -H "Content-type: application/json" -X POST -d '{"loginid":"apple@126.com","password":"1qaz@wsx"}' http://localhost:5001/register
curl -v -H "Content-type: application/json" -X POST http://localhost:5001/health