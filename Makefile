docker:
	docker stop englandsystems >/dev/null 2>&1 ||true;
	docker rm -f englandsystems;
	docker build -t englandsystems:latest .;
	docker run --name englandsystems \
		--env-file ./.env \
		-p 9944:9944 \
		-v /home/phillip/englandsystems/data/englandsystems.sqlite3.db:/data/englandsystems.sqlite3.db \
		englandsystems:latest;

		
