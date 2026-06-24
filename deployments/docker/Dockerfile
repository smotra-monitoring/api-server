FROM ubuntu:latest

COPY ./api-server .
COPY ./configs/prod.html ./configs/

EXPOSE 8080

CMD [ "./api-server", "-c", "./configs/prod.yaml" ]

