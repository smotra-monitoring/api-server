FROM ubuntu:latest

WORKDIR /app

COPY ./api-server .

EXPOSE 8080

CMD [ "./api-server", "-c", "./config/config.yaml" ]

