package api

// return CreateTempateDeployemnt
func (c *Client) CreateTemplateDeployment(orgId string, template map[string]interface{}, vars map[string]string) (*TemplateDeployment, error) {

	query := `
	mutation($input: CreateTemplateDeploymentInput!){
		createTemplateDeployment(input: $input){
			templateDeployment{
		  		id
		  		status
		  		apps{
			  		nodes{
						id
						release{
							id
			  			}
			  			version
			  			deployed
			  			status
					}
		  		}
			}
	  	}
	}
	`

	input := CreateTemplateDeploymentInput{
		OrganizationId: orgId,
		Template:       template,
	}

	for k, v := range vars {
		input.Variables = append(input.Variables, PropertyInput{Name: k, Value: v})
	}

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.CreateTemplateDeployment.TemplateDeployment, nil
}
