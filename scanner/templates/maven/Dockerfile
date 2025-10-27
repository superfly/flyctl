#
# BUILD STAGE
#
FROM maven:3.9.11-sapmachine-25 AS build
COPY src /usr/src/app/src
COPY pom.xml /usr/src/app
RUN mvn -f /usr/src/app/pom.xml clean package

#
# PACKAGE STAGE
#
FROM openjdk:26-slim
# COPY --from=build /usr/src/app/target/demo-0.0.1-SNAPSHOT.jar /usr/app/demo-0.0.1-SNAPSHOT.jar
# EXPOSE 8080
# CMD ["java","-jar","/usr/app/demo-0.0.1-SNAPSHOT.jar"]
COPY --from=build /usr/src/app/target/*.jar /usr/app/app.jar
EXPOSE 8080
CMD ["java","-jar","/usr/app/app.jar"]
