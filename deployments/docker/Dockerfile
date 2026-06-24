FROM ubuntu:latest

COPY ./api-server .
COPY ./configs/prod.yaml ./configs/

EXPOSE 8080

CMD [ "./api-server", "-c", "./configs/prod.yaml" ]

