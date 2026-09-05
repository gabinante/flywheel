package db

// ProjectRepoPredicate compares complete owner/name identities, including dots.
func ProjectRepoPredicate(repoColumn, projectParam string) string {
 return `EXISTS (SELECT 1 FROM (SELECT repo_url FROM projects WHERE id=`+projectParam+` UNION SELECT repo_url FROM project_repositories WHERE project_id=`+projectParam+`) project_repos WHERE repo_url<>'' AND lower(regexp_replace(regexp_replace(repo_url,'^(https://github.com/|git@github.com:|ssh://git@github.com/)','','i'),'[.]git/?$','','i'))=lower(`+repoColumn+`))`
}
