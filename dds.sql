create table if not exists project(
    id char(36) primary key,
    project_url varchar(300),
    webhook_type varchar(20),
    repo_url varchar(200)
)